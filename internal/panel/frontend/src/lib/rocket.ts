/**
 * The flight model and dashed trail behind `NightRocket.svelte`, kept apart
 * from the canvas so the behaviour is testable: everything here is driven by
 * an injected clock (`step(dt)`) and an injected `random`, reads nothing from
 * the DOM, and allocates almost nothing per step.
 *
 * The rocket wanders, and wandering that reads as flight rather than as a
 * screensaver comes down to one rule: nothing is ever set, everything is
 * approached. Speed eases toward a target, the turn rate eases toward a
 * target, and the targets are what the state machine moves - so a stop is a
 * long glide, a launch is a slow build, and every curve the trail records is
 * continuous in both position and curvature.
 */

import { CROSSING_EDGES, pickCrossing, type CrossingEdge } from './crossing';

const TAU = Math.PI * 2;

/** How far past the bounds an entry begins and a departure ends: the whole
 * drawing, flame included, is off the field at both moments. */
const OFFSTAGE = 40;

/** Cruising turn-rate cap - what keeps ordinary wandering to gentle arcs. */
const MAX_TURN = 0.9;
/** How hard the rocket steers back onto its chosen course. */
const COURSE_GAIN = 1.1;
/** How quickly the actual turn rate reaches its target (1/s). */
const TURN_EASE = 2.8;
/** No demanded turn rate passes this, tight turns and walls included. */
const HARD_TURN_CAP = 3.4;
/** Speed approach rates (1/s): the climb is eager, the glide is long. */
const ACCEL_RATE = 0.7;
const BRAKE_RATE = 0.8;
/** Below this the glide is over and the rocket parks (px/s). */
const STOP_SPEED = 2.5;
/** Rotating in place before a launch: gain toward the new course and cap. */
const AIM_GAIN = 1.8;
const AIM_RATE = 1.4;
/**
 * The soft wall. Inside this band the steering toward the centre ramps from
 * nothing to overwhelming - it outweighs every other input rather than
 * replacing it, so the path bends away from the edge instead of kinking.
 */
const EDGE_MARGIN = 110;
const EDGE_GAIN = 2.5;
const EDGE_MAX = 2.4;
/** Seconds of cruising before another rest may be drawn. */
const MIN_FLIGHT = 14;
/**
 * The pace band. Cruise speed is not constant: a fraction of the configured
 * top speed is redrawn at every decision and a slow ripple plays underneath,
 * so the flight has quick stretches and lazy ones. The band and the ripple
 * together still reach only the configured speed, never past it.
 */
const PACE_MIN = 0.55;
const PACE_SPAN = 0.39;
const PACE_RIPPLE = 0.05;

function clamp(value: number, low: number, high: number): number {
  return Math.min(high, Math.max(low, value));
}

/** Shortest signed way round from one angle to another, in (-π, π]. */
function wrapAngle(angle: number): number {
  let wrapped = angle % TAU;
  if (wrapped > Math.PI) wrapped -= TAU;
  if (wrapped <= -Math.PI) wrapped += TAU;
  return wrapped;
}

/**
 * The fraction of the way to a target covered in `dt` at `rate`. Exponential
 * approach: framerate-independent, never overshoots, and slows as it arrives,
 * which is exactly the ease a vehicle's speed and steering want.
 */
function ease(dt: number, rate: number): number {
  return 1 - Math.exp(-dt * rate);
}

export interface TrailPoint {
  x: number;
  y: number;
}

/** One dash of the trail: a short polyline and the moment it began. */
export interface TrailDash {
  born: number;
  pts: TrailPoint[];
}

/**
 * Turns a continuous path into dashes by walking arc length, not time: pen
 * down for `dash` pixels, up for `gap` pixels, whatever the speed and the
 * framerate are doing. Each dash keeps every point it was fed while the pen
 * was down, which is what lets the trail hold the shape of a circle or a
 * tight hook instead of straightening it into chords.
 */
export class TrailEmitter {
  private readonly dash: number;
  private readonly gap: number;
  private readonly cap: number;
  private readonly finished: TrailDash[] = [];
  private current: TrailDash | null = null;
  private down = true;
  private phase = 0;
  private last: TrailPoint | null = null;

  constructor(options: { dash?: number; gap?: number; cap?: number } = {}) {
    this.dash = options.dash ?? 7;
    this.gap = options.gap ?? 6;
    this.cap = options.cap ?? 480;
  }

  /** Completed dashes, oldest first. */
  dashes(): readonly TrailDash[] {
    return this.finished;
  }

  /** The dash still being drawn under the pen, if any. */
  growing(): TrailDash | null {
    return this.current;
  }

  get empty(): boolean {
    return this.finished.length === 0 && this.current === null;
  }

  /**
   * Lift the pen: the current dash is kept if it has any length, and the next
   * `advance` starts a fresh dash at wherever the pen comes down - no segment
   * bridges the jump. This is how a stop ends the trail and a relaunch starts
   * a new one.
   */
  lift(): void {
    if (this.current !== null && this.current.pts.length > 1) {
      this.pushFinished(this.current);
    }
    this.current = null;
    this.last = null;
    this.down = true;
    this.phase = 0;
  }

  /** Move the pen to (x, y), splitting the movement into dashes and gaps. */
  advance(x: number, y: number, time: number): void {
    if (this.last === null) {
      this.last = { x, y };
      this.current = { born: time, pts: [{ x, y }] };
      return;
    }
    let px = this.last.x;
    let py = this.last.y;
    const dx = x - px;
    const dy = y - py;
    let remaining = Math.hypot(dx, dy);
    if (remaining < 1e-6) return;
    const ux = dx / remaining;
    const uy = dy / remaining;
    while (remaining > 0) {
      const span = (this.down ? this.dash : this.gap) - this.phase;
      if (span > remaining) {
        this.phase += remaining;
        if (this.down && this.current !== null) this.current.pts.push({ x, y });
        break;
      }
      px += ux * span;
      py += uy * span;
      remaining -= span;
      if (this.down && this.current !== null) {
        this.current.pts.push({ x: px, y: py });
        this.pushFinished(this.current);
        this.current = null;
      } else {
        this.current = { born: time, pts: [{ x: px, y: py }] };
      }
      this.down = !this.down;
      this.phase = 0;
    }
    this.last = { x, y };
  }

  /** Drop every dash older than `life` seconds. Dashes are born in order, so
   * pruning from the front is complete. */
  prune(time: number, life: number): void {
    while (this.finished.length > 0) {
      const head = this.finished[0];
      if (head === undefined || time - head.born <= life) break;
      this.finished.shift();
    }
  }

  private pushFinished(dash: TrailDash): void {
    this.finished.push(dash);
    // A backstop, not a policy: pruning by age is what normally bounds the
    // trail, but a huge life over a fast rocket must not grow without limit.
    while (this.finished.length > this.cap) this.finished.shift();
  }
}

/** Where the rocket may fly, in the canvas's own coordinates. */
export interface FlightBounds {
  minX: number;
  minY: number;
  maxX: number;
  maxY: number;
}

export interface FlightConfig {
  bounds: FlightBounds;
  /** Cruising speed in px/s. */
  cruise: number;
  /**
   * The edges the rocket may enter and leave through. Not every bounds edge
   * lies off screen - the sky band's bottom sits mid-page - and a rocket
   * must only ever appear and disappear where nobody can watch it do so.
   */
  edges?: readonly CrossingEdge[];
  /** Injected so a test can seed it; the component passes `Math.random`. */
  random: () => number;
}

type FlightMode = 'cruise' | 'brake' | 'idle' | 'aim' | 'launch';

/**
 * One rocket. `step(dt)` advances it; the caller reads position, heading and
 * `thrust` (0..1, what the flame is sized from) to draw it.
 */
export class Flight {
  x: number;
  y: number;
  heading: number;
  speed: number;
  /** How hard the engine is burning, eased, 0 when coasting or parked. */
  thrust = 0;
  /** Still flying in from off stage; steering takes over once inside. */
  entering = true;
  /** Committed to leaving; see `depart`. */
  departing = false;

  private bounds: FlightBounds;
  private cruise: number;
  private readonly edges?: readonly CrossingEdge[];
  private readonly random: () => number;
  private time = 0;
  private mode: FlightMode = 'cruise';
  private course: number;
  private omega = 0;
  private decideIn: number;
  private sinceRest = 0;
  private idleLeft = 0;
  /** A committed arc - a circle or a tight turn: rate, and radians left. */
  private arcLeft = 0;
  private arcOmega = 0;
  /** The fraction of the top speed currently being flown toward. */
  private pace: number;
  /** Where flying is invisible - behind the page's content. See setQuiet. */
  private quiet: FlightBounds | null = null;
  /** Wander phases, fixed at birth so each rocket meanders its own way. */
  private readonly wanderA: number;
  private readonly wanderB: number;

  constructor(cfg: FlightConfig) {
    this.bounds = cfg.bounds;
    this.cruise = Math.max(10, cfg.cruise);
    this.edges = cfg.edges;
    this.random = cfg.random;
    // Born off stage: the rocket flies in from past an allowed edge, on the
    // line a crossing would take, so it never pops into existence where it
    // can be seen. The ordinary steering takes over once it is inside.
    const crossing = pickCrossing({
      width: this.spanX(),
      height: this.spanY(),
      edges: this.edges,
      outside: OFFSTAGE,
      random: this.random,
    });
    this.x = this.bounds.minX + crossing.x;
    this.y = this.bounds.minY + crossing.y;
    this.heading = Math.atan2(crossing.uy, crossing.ux);
    this.course = this.heading;
    this.pace = PACE_MIN + this.random() * PACE_SPAN;
    this.speed = this.cruise * this.pace;
    this.decideIn = 2 + this.random() * 3;
    this.wanderA = this.random() * TAU;
    this.wanderB = this.random() * TAU;
  }

  /** Fully parked: engine off, nothing moving. The caller can stop drawing. */
  get resting(): boolean {
    return this.mode === 'idle';
  }

  setCruise(value: number): void {
    this.cruise = Math.max(10, value);
  }

  setBounds(bounds: FlightBounds): void {
    this.bounds = bounds;
    // A rocket legitimately off stage - entering or leaving - must not be
    // teleported inside by a resize; the clamp is for the ones already in.
    if (this.entering || this.departing) return;
    const m = this.margin();
    this.x = clamp(this.x, bounds.minX + m, Math.max(bounds.minX + m, bounds.maxX - m));
    this.y = clamp(this.y, bounds.minY + m, Math.max(bounds.minY + m, bounds.maxY - m));
  }

  /**
   * Leave the sky: straight for the nearest allowed edge at full burn, every
   * manoeuvre suppressed on the way. Used when a home goes inactive - a
   * theme switch - so the flight ends off screen instead of being cut.
   * `gone` turns true once the whole drawing is past the bounds.
   */
  depart(): void {
    if (this.departing) return;
    this.departing = true;
    this.entering = false;
    this.arcLeft = 0;
    this.course = this.exitCourse();
    // A rocket mid-rest is woken: sitting out a departure is not leaving.
    if (this.mode !== 'cruise') this.mode = 'launch';
  }

  get gone(): boolean {
    return (
      this.departing &&
      (this.x < this.bounds.minX - OFFSTAGE ||
        this.x > this.bounds.maxX + OFFSTAGE ||
        this.y < this.bounds.minY - OFFSTAGE ||
        this.y > this.bounds.maxY + OFFSTAGE)
    );
  }

  /**
   * A region where flying is pointless because nobody can see it - the page's
   * content panel sits over it. The rocket is not kept out: it crosses, but
   * it crosses *straight*, holding the heading it carried in until it is out
   * the other side, and it saves its circles, turns and rests for where they
   * can be watched.
   */
  setQuiet(zone: FlightBounds | null): void {
    this.quiet = zone;
  }

  /**
   * Begin the rest sequence: glide to a stop, sit a while, turn on the spot
   * toward a fresh course, then build back up to cruise. Public so a test can
   * force it; `decide` draws it at random.
   */
  rest(): void {
    if (this.mode !== 'cruise') return;
    this.mode = 'brake';
    this.arcLeft = 0;
    this.sinceRest = 0;
  }

  /** Fly one full circle. Only from cruise, and only if there is room. */
  loop(): void {
    this.startArc(0.85 + this.random() * 0.6, TAU);
  }

  /**
   * A barrel: a sudden tight turn, wrenched round at several times the
   * cruising turn cap and eased back out of, hooking the trail sharply where
   * the ordinary wandering only ever bends it.
   */
  tightTurn(): void {
    this.startArc(2 + this.random() * 1.2, 2.5 + this.random() * 2);
  }

  step(dt: number): void {
    this.time += dt;
    let targetOmega = 0;
    let targetSpeed = 0;

    switch (this.mode) {
      case 'cruise': {
        this.sinceRest += dt;
        this.decideIn -= dt;
        if (this.entering && this.wallDistance() > this.margin()) this.entering = false;
        const straightOnly = this.entering || this.departing || this.hidden();
        if (straightOnly) {
          // Entering, leaving, or behind the panel: hold the course and
          // nothing else. Behind the panel the course *is* the heading it
          // carried in, so that crossing is dead straight; any arc is
          // abandoned, and the next decision waits for open sky.
          if (this.hidden() && !this.departing) this.course = this.heading;
          this.arcLeft = 0;
          if (this.decideIn <= 0) this.decideIn = 0.5;
          targetOmega = clamp(
            wrapAngle(this.course - this.heading) * COURSE_GAIN,
            -MAX_TURN,
            MAX_TURN,
          );
        } else {
          if (this.decideIn <= 0) this.decide();
          if (this.arcLeft > 0 && this.wallDistance() < 60) {
            // The arc has drifted too close to a wall; give it up and let
            // the ordinary steering carry the rocket back inside.
            this.arcLeft = 0;
            this.course = this.centreward();
          }
          if (this.arcLeft > 0) {
            targetOmega = this.arcOmega;
            this.arcLeft -= Math.abs(this.omega) * dt;
            // An arc ends on whatever heading it reaches, and that heading
            // is the course now - a circle hands back the old one.
            if (this.arcLeft <= 0) this.course = this.heading;
          } else {
            targetOmega =
              clamp(wrapAngle(this.course - this.heading) * COURSE_GAIN, -MAX_TURN, MAX_TURN) +
              this.wander();
          }
        }
        // The soft wall stays on except while leaving - it would fight the
        // exit - and entry welcomes it: it bends the arrival inward.
        if (!this.departing) targetOmega += this.edgeSteer();
        targetOmega = clamp(targetOmega, -HARD_TURN_CAP, HARD_TURN_CAP);
        // The rippled pace, and on top of it a tight turn bleeds a little
        // speed, the way a hard bank does; both come back on their own. A
        // departure burns at the full configured speed - it is leaving.
        const pace = clamp(
          this.pace + PACE_RIPPLE * Math.sin(this.time * 0.23 + this.wanderB * 2.1),
          PACE_MIN - PACE_RIPPLE,
          1,
        );
        targetSpeed = this.departing ? this.cruise : this.cruise * pace;
        if (this.arcLeft > 0 && Math.abs(this.arcOmega) > 1.6) targetSpeed *= 0.8;
        break;
      }
      case 'brake': {
        // Engine off, drifting nearly straight while the speed bleeds away -
        // and dead straight where the panel hides it.
        targetOmega = this.hidden() ? 0 : this.wander() * 0.25;
        if (this.speed < STOP_SPEED) {
          this.speed = 0;
          this.omega = 0;
          this.mode = 'idle';
          this.idleLeft = 1.5 + this.random() * 4.5;
        }
        break;
      }
      case 'idle': {
        this.idleLeft -= dt;
        if (this.idleLeft <= 0) {
          this.mode = 'aim';
          this.course = this.pickCourse();
        }
        break;
      }
      case 'aim': {
        const delta = wrapAngle(this.course - this.heading);
        targetOmega = clamp(delta * AIM_GAIN, -AIM_RATE, AIM_RATE);
        if (Math.abs(delta) < 0.05 && Math.abs(this.omega) < 0.1) this.mode = 'launch';
        break;
      }
      case 'launch': {
        targetSpeed = this.cruise * (this.departing ? 1 : this.pace);
        // A launch that carries the rocket behind the panel goes straight
        // there like every other crossing of it - a departure excepted, its
        // course already is the way out.
        if (this.hidden() && !this.departing) this.course = this.heading;
        targetOmega =
          clamp(wrapAngle(this.course - this.heading) * COURSE_GAIN, -MAX_TURN, MAX_TURN) * 0.5;
        if (this.speed >= this.cruise * this.pace * 0.92) {
          this.mode = 'cruise';
          this.decideIn = 2.5 + this.random() * 4.5;
        }
        break;
      }
    }

    this.omega += (targetOmega - this.omega) * ease(dt, TURN_EASE);
    this.speed +=
      (targetSpeed - this.speed) * ease(dt, targetSpeed > this.speed ? ACCEL_RATE : BRAKE_RATE);

    this.heading = wrapAngle(this.heading + this.omega * dt);
    this.x += Math.cos(this.heading) * this.speed * dt;
    this.y += Math.sin(this.heading) * this.speed * dt;
    this.confine();

    // The flame: some for moving at all, much more while the speed is still
    // being built - a launch burns hard, a steady cruise burns evenly.
    const burning = this.mode === 'cruise' || this.mode === 'launch';
    const demand = Math.max(0, targetSpeed - this.speed) / this.cruise;
    const targetThrust = burning ? clamp(0.5 * (this.speed / this.cruise) + 0.9 * demand, 0, 1) : 0;
    this.thrust += (targetThrust - this.thrust) * ease(dt, 6);
  }

  private startArc(omega: number, angle: number): void {
    if (this.mode !== 'cruise' || this.arcLeft > 0) return;
    const direction = this.random() < 0.5 ? -1 : 1;
    // The arc's whole sweep stays clear of the walls or it is not flown: its
    // far side sits a diameter away, and the soft wall would squash the shape
    // the trail is about to record.
    const room = 2.3 * (this.cruise / omega);
    if (this.wallDistance() < Math.min(room, Math.min(this.spanX(), this.spanY()) / 2)) return;
    this.arcOmega = direction * omega;
    this.arcLeft = angle;
  }

  private decide(): void {
    this.decideIn = 2.5 + this.random() * 4.5;
    if (this.arcLeft > 0) {
      // Mid-arc is no time to change plans; look again just after it closes.
      this.decideIn = 0.5;
      return;
    }
    // A fresh pace with every fresh mind: the speed drifts to it under the
    // ordinary easing, so a change of pace is a mood, not a gear shift.
    this.pace = PACE_MIN + this.random() * PACE_SPAN;
    const r = this.random();
    if (r < 0.16 && this.sinceRest > MIN_FLIGHT) {
      this.rest();
    } else if (r < 0.34) {
      this.loop();
    } else if (r < 0.55) {
      this.tightTurn();
    } else {
      this.course = this.pickCourse();
    }
  }

  /**
   * A new course is drawn around the direction of the centre, not around the
   * current heading: courses that only ever bend a little from wherever the
   * rocket points let it grind along an edge, while centre-biased ones keep
   * it roaming the interior without ever aiming it anywhere twice.
   */
  private pickCourse(): number {
    return wrapAngle(this.centreward() + (this.random() - 0.5) * 2.8);
  }

  private centreward(): number {
    return Math.atan2(
      (this.bounds.minY + this.bounds.maxY) / 2 - this.y,
      (this.bounds.minX + this.bounds.maxX) / 2 - this.x,
    );
  }

  /** Slow meander summed from two incommensurate sines: cheap, smooth, and
   * never repeats within a sitting. */
  private wander(): number {
    return (
      0.22 * Math.sin(this.time * 0.47 + this.wanderA) +
      0.13 * Math.sin(this.time * 1.13 + this.wanderB)
    );
  }

  private spanX(): number {
    return this.bounds.maxX - this.bounds.minX;
  }

  private spanY(): number {
    return this.bounds.maxY - this.bounds.minY;
  }

  private hidden(): boolean {
    const q = this.quiet;
    return q !== null && this.x > q.minX && this.x < q.maxX && this.y > q.minY && this.y < q.maxY;
  }

  private wallDistance(): number {
    return Math.min(
      this.x - this.bounds.minX,
      this.bounds.maxX - this.x,
      this.y - this.bounds.minY,
      this.bounds.maxY - this.y,
    );
  }

  private edgeSteer(): number {
    const m = Math.min(EDGE_MARGIN, this.spanX() * 0.3, this.spanY() * 0.3);
    const nearest = this.wallDistance();
    if (nearest >= m) return 0;
    const urgency = 1 - nearest / m;
    return clamp(
      wrapAngle(this.centreward() - this.heading) * EDGE_GAIN * urgency,
      -EDGE_MAX,
      EDGE_MAX,
    );
  }

  /**
   * The hard backstop behind the soft wall. Position is clamped but heading
   * is not touched: a heading jump would put a kink in the trail, so the
   * course is pointed inward and the steering does the turning.
   */
  private confine(): void {
    // Off stage on purpose: an entry has not arrived yet and a departure is
    // exactly the act of leaving - clamping either would teleport it.
    if (this.entering || this.departing) return;
    const m = this.margin();
    const cx = clamp(
      this.x,
      this.bounds.minX + m,
      Math.max(this.bounds.minX + m, this.bounds.maxX - m),
    );
    const cy = clamp(
      this.y,
      this.bounds.minY + m,
      Math.max(this.bounds.minY + m, this.bounds.maxY - m),
    );
    if (cx !== this.x || cy !== this.y) {
      this.x = cx;
      this.y = cy;
      this.course = this.centreward();
    }
  }

  private margin(): number {
    // Room for the artwork itself - nose, flame and trail all stay inside
    // the canvas even with the hull pinned to the clamp.
    return Math.min(26, this.spanX() / 4, this.spanY() / 4);
  }

  /** Perpendicular course through the nearest edge a flight may leave by. */
  private exitCourse(): number {
    const edges = this.edges === undefined || this.edges.length === 0 ? CROSSING_EDGES : this.edges;
    let best: CrossingEdge = edges[0] ?? 'left';
    let bestDistance = Infinity;
    for (const edge of edges) {
      const distance =
        edge === 'left'
          ? this.x - this.bounds.minX
          : edge === 'right'
            ? this.bounds.maxX - this.x
            : edge === 'top'
              ? this.y - this.bounds.minY
              : this.bounds.maxY - this.y;
      if (distance < bestDistance) {
        bestDistance = distance;
        best = edge;
      }
    }
    if (best === 'left') return Math.PI;
    if (best === 'right') return 0;
    if (best === 'top') return -Math.PI / 2;
    return Math.PI / 2;
  }
}

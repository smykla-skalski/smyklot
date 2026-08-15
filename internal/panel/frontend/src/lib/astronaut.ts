/**
 * The drift behind `NightAstronaut.svelte`, kept apart from the canvas for
 * the same reason as `rocket.ts`: driven by an injected clock and an
 * injected `random`, it reads nothing from the DOM and every promise about
 * the motion is provable in a unit test.
 *
 * An astronaut adrift is the opposite of the rocket: no engine, no will, no
 * steering. One crossing is decided entirely at birth - where it enters,
 * where it leaves, how fast it goes and how fast it tumbles - and then
 * nothing changes until it is gone. The listlessness *is* the constancy.
 */

import { pickCrossing, type CrossingEdge } from './crossing';

/** How far outside the field a crossing starts and ends, so the figure is
 * entirely off the canvas at both ends of its journey. */
const OUTSIDE = 30;
/** A crossing takes this long, whatever distance it spans... */
const CROSS_MIN_S = 18;
const CROSS_SPAN_S = 32;
/** ...within these speeds, so a short hop is not a dart and a long diagonal
 * is not a crawl (px/s). */
const SPEED_MIN = 8;
const SPEED_MAX = 45;
/** Tumble rates, rad/s: always turning, never spinning. */
const SPIN_MIN = 0.15;
const SPIN_SPAN = 0.75;

const TAU = Math.PI * 2;

function clamp(value: number, low: number, high: number): number {
  return Math.min(high, Math.max(low, value));
}

export interface DriftConfig {
  /** The field being crossed, in the canvas's CSS pixels. */
  width: number;
  height: number;
  /** The edges a crossing may begin and end past - see `CrossingConfig`. */
  edges?: readonly CrossingEdge[];
  /** Injected so a test can seed it; the component passes `Math.random`. */
  random: () => number;
}

/**
 * One crossing: spawned outside a random edge, drifting in a straight line
 * at constant speed to a point outside a different edge, tumbling at a
 * constant rate the whole way. `step(dt)` advances it; the caller reads
 * `x`, `y` and `angle` to draw, and `done` to know the field is empty.
 */
export class Drift {
  x: number;
  y: number;
  /** The tumble, free-running - the figure rolls around itself. */
  angle: number;
  done = false;

  readonly entryEdge: CrossingEdge;
  readonly exitEdge: CrossingEdge;
  readonly speed: number;
  readonly spin: number;

  private readonly vx: number;
  private readonly vy: number;
  private readonly length: number;
  private travelled = 0;

  constructor(cfg: DriftConfig) {
    const random = cfg.random;
    const crossing = pickCrossing({
      width: cfg.width,
      height: cfg.height,
      edges: cfg.edges,
      outside: OUTSIDE,
      random,
    });
    this.x = crossing.x;
    this.y = crossing.y;
    this.length = crossing.length;
    this.entryEdge = crossing.entryEdge;
    this.exitEdge = crossing.exitEdge;
    const duration = CROSS_MIN_S + random() * CROSS_SPAN_S;
    this.speed = clamp(this.length / duration, SPEED_MIN, SPEED_MAX);
    this.vx = crossing.ux * this.speed;
    this.vy = crossing.uy * this.speed;
    this.spin = (random() < 0.5 ? -1 : 1) * (SPIN_MIN + random() * SPIN_SPAN);
    this.angle = random() * TAU;
  }

  step(dt: number): void {
    if (this.done) return;
    this.x += this.vx * dt;
    this.y += this.vy * dt;
    this.angle += this.spin * dt;
    this.travelled += this.speed * dt;
    if (this.travelled >= this.length) this.done = true;
  }
}

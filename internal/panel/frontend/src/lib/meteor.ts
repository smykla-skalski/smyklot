/**
 * The streak model behind `NightMeteors.svelte`, pure and injected like the
 * others: a meteor is a crossing at speed - the same outside-to-outside
 * straight line the astronaut drifts, burned through in a couple of seconds
 * with a tail streaming behind the head.
 *
 * The tail is why the crossing's `outside` margin is the tail's own length
 * plus clearance: the head may only despawn once the last of the tail has
 * cleared the field, because nothing here is allowed to vanish where it can
 * be seen.
 */

import { pickCrossing, type CrossingEdge } from './crossing';

/** Fast, but not a flicker: a whole crossing takes a second or three. */
const SPEED_MIN = 260;
const SPEED_SPAN = 260;
/** How far the burn stretches behind the head. */
const TAIL_MIN = 55;
const TAIL_SPAN = 75;
/** Past the tail, a little room so no despawn is ever on the line. */
const CLEARANCE = 30;

export interface StreakConfig {
  /** The field being crossed, in the canvas's CSS pixels. */
  width: number;
  height: number;
  /** The edges a streak may begin and end past - see `CrossingConfig`. */
  edges?: readonly CrossingEdge[];
  /** Injected so a test can seed it; the component passes `Math.random`. */
  random: () => number;
}

/** One meteor: a straight, constant burn across the field. */
export class Streak {
  x: number;
  y: number;
  done = false;

  readonly ux: number;
  readonly uy: number;
  readonly speed: number;
  readonly tail: number;
  readonly entryEdge: CrossingEdge;
  readonly exitEdge: CrossingEdge;

  private travelled = 0;
  private readonly length: number;

  constructor(cfg: StreakConfig) {
    const random = cfg.random;
    this.tail = TAIL_MIN + random() * TAIL_SPAN;
    const crossing = pickCrossing({
      width: cfg.width,
      height: cfg.height,
      edges: cfg.edges,
      outside: this.tail + CLEARANCE,
      random,
    });
    this.x = crossing.x;
    this.y = crossing.y;
    this.ux = crossing.ux;
    this.uy = crossing.uy;
    this.length = crossing.length;
    this.entryEdge = crossing.entryEdge;
    this.exitEdge = crossing.exitEdge;
    this.speed = SPEED_MIN + random() * SPEED_SPAN;
  }

  step(dt: number): void {
    if (this.done) return;
    this.x += this.ux * this.speed * dt;
    this.y += this.uy * this.speed * dt;
    this.travelled += this.speed * dt;
    if (this.travelled >= this.length) this.done = true;
  }
}

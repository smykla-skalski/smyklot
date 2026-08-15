/**
 * The shared geometry of a crossing: a straight line from a point outside
 * one edge of a field to a point outside another. The astronaut drifts one
 * slowly; a meteor burns one fast. What they share is the promise that the
 * journey begins and ends off the field entirely - nothing pops into or out
 * of existence where it can be seen.
 */

export type CrossingEdge = 'left' | 'right' | 'top' | 'bottom';

export const CROSSING_EDGES: readonly CrossingEdge[] = ['left', 'right', 'top', 'bottom'];

/** Entry and exit keep off the corners: a crossing pinned to a corner cuts
 * it without ever really being seen. */
const EDGE_KEEP = 0.15;

export interface CrossingConfig {
  /** The field being crossed, in the canvas's CSS pixels. */
  width: number;
  height: number;
  /**
   * The edges a crossing may begin and end past. Not every canvas edge lies
   * off screen - the sky band's bottom sits mid-page, so its caller leaves
   * `bottom` out. Fewer than two falls back to all four; there has to be
   * somewhere to go.
   */
  edges?: readonly CrossingEdge[];
  /** How far past the edges the crossing begins and ends. A body with a
   * tail passes its tail's length here, so the tail clears too. */
  outside: number;
  random: () => number;
}

export interface Crossing {
  /** Where it begins, outside the entry edge. */
  x: number;
  y: number;
  /** Unit direction of travel. */
  ux: number;
  uy: number;
  /** Full distance, outside point to outside point. */
  length: number;
  entryEdge: CrossingEdge;
  exitEdge: CrossingEdge;
}

export function pickCrossing(cfg: CrossingConfig): Crossing {
  const random = cfg.random;
  const allowed = cfg.edges === undefined || cfg.edges.length < 2 ? CROSSING_EDGES : cfg.edges;
  const entryIndex = Math.floor(random() * allowed.length) % allowed.length;
  const entryEdge = allowed[entryIndex] ?? 'left';
  const others = allowed.filter((edge) => edge !== entryEdge);
  const exitEdge = others[Math.floor(random() * others.length) % others.length] ?? 'right';
  const from = edgePoint(entryEdge, cfg, random());
  const to = edgePoint(exitEdge, cfg, random());
  const dx = to.x - from.x;
  const dy = to.y - from.y;
  const length = Math.hypot(dx, dy);
  return {
    x: from.x,
    y: from.y,
    ux: dx / length,
    uy: dy / length,
    length,
    entryEdge,
    exitEdge,
  };
}

function edgePoint(
  edge: CrossingEdge,
  cfg: CrossingConfig,
  along: number,
): { x: number; y: number } {
  const span = EDGE_KEEP + along * (1 - 2 * EDGE_KEEP);
  switch (edge) {
    case 'left':
      return { x: -cfg.outside, y: span * cfg.height };
    case 'right':
      return { x: cfg.width + cfg.outside, y: span * cfg.height };
    case 'top':
      return { x: span * cfg.width, y: -cfg.outside };
    case 'bottom':
      return { x: span * cfg.width, y: cfg.height + cfg.outside };
  }
}

import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import { Drift } from '../src/lib/astronaut';

/**
 * A crossing is a straight, constant drift decided entirely at birth. What
 * is provable: it starts outside the field, passes through it, leaves it on
 * a different side, and neither its speed nor its tumble ever changes on the
 * way - listlessness as an invariant.
 */

/** mulberry32 - the same seedable generator the rocket's tests use. */
function rng(seed: number): () => number {
  let s = seed >>> 0;
  return () => {
    s = (s + 0x6d2b79f5) >>> 0;
    let t = s;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

describe('Drift [Unit]', () => {
  const width = 1200;
  const height = 640;
  const dt = 1 / 60;

  const inside = (d: Drift): boolean => d.x >= 0 && d.x <= width && d.y >= 0 && d.y <= height;

  it('enters from outside, crosses the field, and leaves by another edge', () => {
    for (let seed = 1; seed <= 40; seed += 1) {
      const drift = new Drift({ width, height, random: rng(seed) });
      expect(inside(drift)).toBe(false);
      expect(drift.exitEdge).not.toBe(drift.entryEdge);
      let wasInside = false;
      let steps = 0;
      while (!drift.done) {
        drift.step(dt);
        if (inside(drift)) wasInside = true;
        steps += 1;
        // Longest legal crossing: the whole diagonal at the floor speed.
        expect(steps).toBeLessThan(60 * 240);
      }
      expect(wasInside).toBe(true);
      expect(inside(drift)).toBe(false);
    }
  });

  it('drifts at one constant speed, inside the advertised band', () => {
    const drift = new Drift({ width, height, random: rng(9) });
    expect(drift.speed).toBeGreaterThanOrEqual(8);
    expect(drift.speed).toBeLessThanOrEqual(45);
    let lastX = drift.x;
    let lastY = drift.y;
    while (!drift.done) {
      drift.step(dt);
      const moved = Math.hypot(drift.x - lastX, drift.y - lastY);
      expect(moved).toBeCloseTo(drift.speed * dt, 9);
      lastX = drift.x;
      lastY = drift.y;
    }
  });

  it('tumbles at one constant rate, always turning, never spinning', () => {
    const drift = new Drift({ width, height, random: rng(17) });
    expect(Math.abs(drift.spin)).toBeGreaterThanOrEqual(0.15);
    expect(Math.abs(drift.spin)).toBeLessThanOrEqual(0.9);
    let lastAngle = drift.angle;
    for (let i = 0; i < 60 * 20 && !drift.done; i += 1) {
      drift.step(dt);
      expect(drift.angle - lastAngle).toBeCloseTo(drift.spin * dt, 9);
      lastAngle = drift.angle;
    }
  });

  it('keeps to the allowed edges - a crossing may not use one that lies mid-page', () => {
    for (let seed = 1; seed <= 30; seed += 1) {
      const drift = new Drift({
        width,
        height,
        edges: ['left', 'right', 'top'],
        random: rng(seed),
      });
      expect(drift.entryEdge).not.toBe('bottom');
      expect(drift.exitEdge).not.toBe('bottom');
      expect(drift.exitEdge).not.toBe(drift.entryEdge);
    }
  });

  it('goes nowhere further once done', () => {
    const drift = new Drift({ width, height, random: rng(23) });
    while (!drift.done) drift.step(dt);
    const { x, y, angle } = drift;
    drift.step(dt);
    expect(drift.x).toBe(x);
    expect(drift.y).toBe(y);
    expect(drift.angle).toBe(angle);
  });
});

/**
 * The component's safety promises, checked as source like the rocket's: the
 * loop must be cancellable, the scheduler's timer must be torn down, the
 * reduced-motion gate must live in JS (the CSS squash cannot reach a
 * canvas), off screen must stop it, and no `style` attribute may appear for
 * the panel's CSP to discard.
 */
describe('NightAstronaut [Unit]', () => {
  const source = readFileSync(
    new URL('../src/components/NightAstronaut.svelte', import.meta.url),
    'utf8',
  );

  it('cancels its animation frame on teardown', () => {
    expect(source).toContain('cancelAnimationFrame');
  });

  it('clears its appearance timer on teardown', () => {
    expect(source).toContain('clearTimeout');
  });

  it('gates itself on prefers-reduced-motion where CSS cannot', () => {
    expect(source).toContain('prefers-reduced-motion');
  });

  it('stops when scrolled out of view', () => {
    expect(source).toContain('IntersectionObserver');
  });

  it('carries no style attribute for the CSP to discard', () => {
    expect(source).not.toMatch(/\sstyle="/u);
  });
});

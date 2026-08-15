import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import { Streak } from '../src/lib/meteor';

/**
 * A meteor is a crossing at speed: straight, constant, decided at birth.
 * What is provable: it starts outside the field, passes through it, and is
 * only done once its whole tail has cleared the far side - plus the same
 * component safety promises the other easter eggs pin.
 */

/** mulberry32 - the same seedable generator the other tests use. */
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

describe('Streak [Unit]', () => {
  const width = 1200;
  const height = 640;
  const dt = 1 / 60;

  const inside = (s: Streak): boolean => s.x >= 0 && s.x <= width && s.y >= 0 && s.y <= height;

  it('burns from outside the field, through it, and out past another edge', () => {
    for (let seed = 1; seed <= 40; seed += 1) {
      const streak = new Streak({ width, height, random: rng(seed) });
      expect(inside(streak)).toBe(false);
      expect(streak.exitEdge).not.toBe(streak.entryEdge);
      let wasInside = false;
      let steps = 0;
      while (!streak.done) {
        streak.step(dt);
        if (inside(streak)) wasInside = true;
        // The longest legal crossing at the floor speed is well under this.
        expect((steps += 1)).toBeLessThan(60 * 10);
      }
      expect(wasInside).toBe(true);
      // Done means the whole burn has cleared: the head rests at least its
      // own tail's length past the field, so the tail is out with it.
      const beyond = Math.max(0 - streak.x, streak.x - width, 0 - streak.y, streak.y - height);
      expect(beyond).toBeGreaterThanOrEqual(streak.tail);
    }
  });

  it('flies at one constant speed, inside the advertised band', () => {
    const streak = new Streak({ width, height, random: rng(9) });
    expect(streak.speed).toBeGreaterThanOrEqual(260);
    expect(streak.speed).toBeLessThanOrEqual(520);
    let lastX = streak.x;
    let lastY = streak.y;
    while (!streak.done) {
      streak.step(dt);
      const moved = Math.hypot(streak.x - lastX, streak.y - lastY);
      expect(moved).toBeCloseTo(streak.speed * dt, 9);
      lastX = streak.x;
      lastY = streak.y;
    }
  });

  it('keeps to the allowed edges - none that lie mid-page', () => {
    for (let seed = 1; seed <= 30; seed += 1) {
      const streak = new Streak({
        width,
        height,
        edges: ['left', 'right', 'top'],
        random: rng(seed),
      });
      expect(streak.entryEdge).not.toBe('bottom');
      expect(streak.exitEdge).not.toBe('bottom');
    }
  });

  it('goes nowhere further once done', () => {
    const streak = new Streak({ width, height, random: rng(23) });
    while (!streak.done) streak.step(dt);
    const { x, y } = streak;
    streak.step(dt);
    expect(streak.x).toBe(x);
    expect(streak.y).toBe(y);
  });
});

/** The component's safety promises, checked as source like the others'. */
describe('NightMeteors [Unit]', () => {
  const source = readFileSync(
    new URL('../src/components/NightMeteors.svelte', import.meta.url),
    'utf8',
  );

  it('cancels its animation frame on teardown', () => {
    expect(source).toContain('cancelAnimationFrame');
  });

  it('clears its shower timer on teardown', () => {
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

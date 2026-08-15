import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import { Flight, TrailEmitter, type TrailDash } from '../src/lib/rocket';

/**
 * The rocket's behaviour is a contract about smoothness: nothing teleports,
 * nothing snaps, the trail records the true path and forgets it on schedule.
 * The model takes an injected `random` and an injected clock so all of that
 * is provable here, deterministically, without a canvas.
 */

const TAU = Math.PI * 2;

/** mulberry32 - a tiny seedable generator, enough to make a flight replayable. */
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

function dashLength(dash: TrailDash): number {
  let length = 0;
  for (let i = 1; i < dash.pts.length; i += 1) {
    const a = dash.pts[i - 1];
    const b = dash.pts[i];
    if (a === undefined || b === undefined) continue;
    length += Math.hypot(b.x - a.x, b.y - a.y);
  }
  return length;
}

describe('TrailEmitter [Unit]', () => {
  it('alternates dashes and gaps by arc length, whatever the step size', () => {
    const trail = new TrailEmitter({ dash: 7, gap: 6 });
    for (let x = 0; x <= 200; x += 1.7) trail.advance(x, 0, 0);
    const dashes = trail.dashes();
    expect(dashes.length).toBeGreaterThan(10);
    for (const dash of dashes) {
      expect(dashLength(dash)).toBeCloseTo(7, 6);
    }
    for (let i = 1; i < dashes.length; i += 1) {
      const prev = dashes[i - 1];
      const next = dashes[i];
      if (prev === undefined || next === undefined) continue;
      const tail = prev.pts[prev.pts.length - 1];
      const head = next.pts[0];
      if (tail === undefined || head === undefined) continue;
      expect(Math.hypot(head.x - tail.x, head.y - tail.y)).toBeCloseTo(6, 6);
    }
  });

  it('keeps the shape of a curved path instead of straightening it', () => {
    const trail = new TrailEmitter({ dash: 7, gap: 6 });
    const radius = 50;
    for (let theta = 0; theta <= Math.PI; theta += 0.02) {
      trail.advance(Math.cos(theta) * radius, Math.sin(theta) * radius, 0);
    }
    for (const dash of trail.dashes()) {
      for (const p of dash.pts) {
        // Every recorded point sits on the circle the pen actually flew.
        expect(Math.abs(Math.hypot(p.x, p.y) - radius)).toBeLessThan(0.75);
      }
    }
  });

  it('drops a dash once it outlives the configured time, and no sooner', () => {
    const trail = new TrailEmitter({ dash: 7, gap: 6 });
    for (let x = 0; x <= 40; x += 2) trail.advance(x, 0, 1);
    for (let x = 42; x <= 80; x += 2) trail.advance(x, 0, 5);
    const before = trail.dashes().length;
    trail.prune(6, 7);
    expect(trail.dashes().length).toBe(before);
    trail.prune(8.5, 7);
    const left = trail.dashes();
    expect(left.length).toBeLessThan(before);
    for (const dash of left) {
      expect(8.5 - dash.born).toBeLessThanOrEqual(7);
    }
  });

  it('holds the dash count at its cap however far the pen travels', () => {
    const trail = new TrailEmitter({ dash: 7, gap: 6, cap: 10 });
    for (let x = 0; x <= 5000; x += 3) trail.advance(x, 0, 0);
    expect(trail.dashes().length).toBeLessThanOrEqual(10);
  });

  it('starts a fresh line after the pen lifts - a stop leaves no bridge', () => {
    const trail = new TrailEmitter({ dash: 7, gap: 6 });
    for (let x = 0; x <= 50; x += 2) trail.advance(x, 0, 0);
    trail.lift();
    for (let x = 300; x <= 360; x += 2) trail.advance(x, 0, 1);
    for (const dash of trail.dashes()) {
      for (const p of dash.pts) {
        expect(p.x <= 51 || p.x >= 299).toBe(true);
      }
    }
  });
});

describe('Flight [Unit]', () => {
  const width = 1200;
  const height = 520;
  const bounds = { minX: 0, minY: 0, maxX: width, maxY: height };
  const cruise = 70;
  const dt = 1 / 60;

  /** Entry is part of every flight now; run it out before asserting on the
   * roaming that follows. */
  function enter(fl: Flight): void {
    let guard = 0;
    while (fl.entering) {
      fl.step(dt);
      expect((guard += 1)).toBeLessThan(60 * 30);
    }
  }

  it('flies in from off stage instead of appearing mid-sky', () => {
    const fl = new Flight({ bounds, cruise, random: rng(41) });
    expect(fl.entering).toBe(true);
    expect(fl.x < 0 || fl.x > width || fl.y < 0 || fl.y > height).toBe(true);
    enter(fl);
    expect(fl.x).toBeGreaterThanOrEqual(0);
    expect(fl.x).toBeLessThanOrEqual(width);
    expect(fl.y).toBeGreaterThanOrEqual(0);
    expect(fl.y).toBeLessThanOrEqual(height);
  });

  it('departs off stage at full burn when told to leave', () => {
    const fl = new Flight({ bounds, cruise, random: rng(43) });
    enter(fl);
    for (let i = 0; i < 60 * 5; i += 1) fl.step(dt);
    fl.depart();
    expect(fl.departing).toBe(true);
    let guard = 0;
    while (!fl.gone) {
      fl.step(dt);
      // Leaving is flying, never sitting: no rest on the way out.
      expect(fl.resting).toBe(false);
      expect((guard += 1)).toBeLessThan(60 * 40);
    }
    const beyond = Math.max(0 - fl.x, fl.x - width, 0 - fl.y, fl.y - height);
    expect(beyond).toBeGreaterThan(39);
  });

  it('stays inside its bounds for minutes of wandering once entered', () => {
    const fl = new Flight({ bounds, cruise, random: rng(7) });
    enter(fl);
    for (let i = 0; i < 240 * 60; i += 1) {
      fl.step(dt);
      expect(fl.x).toBeGreaterThanOrEqual(0);
      expect(fl.x).toBeLessThanOrEqual(width);
      expect(fl.y).toBeGreaterThanOrEqual(0);
      expect(fl.y).toBeLessThanOrEqual(height);
    }
  });

  it('never teleports and never snaps its heading', () => {
    const fl = new Flight({ bounds, cruise, random: rng(11) });
    let lastX = fl.x;
    let lastY = fl.y;
    let lastHeading = fl.heading;
    for (let i = 0; i < 120 * 60; i += 1) {
      fl.step(dt);
      const moved = Math.hypot(fl.x - lastX, fl.y - lastY);
      expect(moved).toBeLessThanOrEqual(cruise * dt + 1e-9);
      let turned = (fl.heading - lastHeading) % TAU;
      if (turned > Math.PI) turned -= TAU;
      if (turned < -Math.PI) turned += TAU;
      expect(Math.abs(turned)).toBeLessThanOrEqual(4 * dt);
      lastX = fl.x;
      lastY = fl.y;
      lastHeading = fl.heading;
    }
  });

  it('rests the way a vehicle does: glide down, sit, turn, build back up', () => {
    const fl = new Flight({ bounds, cruise, random: rng(3) });
    enter(fl);
    fl.rest();
    // The glide: speed only ever falls until the rocket is parked.
    let guard = 0;
    let last = fl.speed;
    while (fl.speed > 0) {
      fl.step(dt);
      expect(fl.speed).toBeLessThanOrEqual(last + 1e-9);
      last = fl.speed;
      expect((guard += 1)).toBeLessThan(60 * 30);
    }
    expect(fl.resting).toBe(true);
    expect(fl.thrust).toBeLessThan(0.05);
    // The sit, the turn, and then the climb: once speed leaves zero it only
    // ever rises while the launch burns. The pace it climbs toward is drawn
    // at random from well above this line, so stopping the assertion here
    // keeps it about the launch, not about the pace of the day.
    guard = 0;
    while (fl.speed === 0) {
      fl.step(dt);
      expect((guard += 1)).toBeLessThan(60 * 30);
    }
    guard = 0;
    last = fl.speed;
    while (fl.speed < cruise * 0.45) {
      fl.step(dt);
      expect(fl.speed).toBeGreaterThanOrEqual(last - 1e-9);
      last = fl.speed;
      expect((guard += 1)).toBeLessThan(60 * 30);
    }
  });

  it('varies its pace but never passes the configured top speed', () => {
    const fl = new Flight({ bounds, cruise, random: rng(21) });
    const burning: number[] = [];
    for (let i = 0; i < 180 * 60; i += 1) {
      fl.step(dt);
      expect(fl.speed).toBeLessThanOrEqual(cruise + 1e-9);
      // Sampled only under thrust, so brake glides and idles do not stand in
      // for pace: the claim is that *cruising* itself speeds up and slows.
      if (fl.thrust > 0.22 && i % 60 === 0) burning.push(fl.speed);
    }
    expect(burning.length).toBeGreaterThan(60);
    const lo = Math.min(...burning);
    const hi = Math.max(...burning);
    expect(hi - lo).toBeGreaterThan(cruise * 0.15);
  });

  it('wrenches a barrel - a tight turn - far sharper than any cruise arc', () => {
    const fl = new Flight({ bounds, cruise, random: rng(5) });
    // Parked in the middle so the turn has the room it asks for.
    fl.x = width / 2;
    fl.y = height / 2;
    fl.tightTurn();
    let peakRate = 0;
    let net = 0;
    let lastHeading = fl.heading;
    for (let i = 0; i < 60 * 3; i += 1) {
      fl.step(dt);
      let turned = (fl.heading - lastHeading) % TAU;
      if (turned > Math.PI) turned -= TAU;
      if (turned < -Math.PI) turned += TAU;
      net += turned;
      peakRate = Math.max(peakRate, Math.abs(turned) / dt);
      lastHeading = fl.heading;
    }
    // Sharper than the 0.9 rad/s cruising cap by a wide margin...
    expect(peakRate).toBeGreaterThan(1.6);
    // ...and sweeping well past a right angle while it lasts.
    expect(Math.abs(net)).toBeGreaterThan(2);
  });

  it('crosses a quiet zone dead straight, on the heading it carried in', () => {
    const fl = new Flight({ bounds, cruise, random: rng(31) });
    // A full-height band across the middle: the rocket cannot roam without
    // crossing it, and inside it every manoeuvre is suspended.
    const zone = { minX: 450, minY: 0, maxX: 780, maxY: height };
    fl.setQuiet(zone);
    const inZone = (): boolean => fl.x > zone.minX && fl.x < zone.maxX;
    let entryHeading: number | null = null;
    let settle = 0;
    for (let i = 0; i < 60 * 240; i += 1) {
      fl.step(dt);
      if (!inZone()) {
        entryHeading = null;
        settle = 0;
        continue;
      }
      // The wall band steers even where flying is hidden, because a crash
      // is worse than a wiggle - so a band excursion restarts the clock the
      // way entering does, giving the eased-out steering time to die.
      if (fl.y < 120 || fl.y > height - 120) {
        settle = 0;
        entryHeading = fl.heading;
        continue;
      }
      // The turn rate is eased, not snapped, so the first moments inside
      // still carry the curve it entered with; after that it holds course.
      settle += dt;
      if (settle < 1.1) {
        entryHeading = fl.heading;
        continue;
      }
      if (entryHeading !== null && fl.x > zone.minX + 40 && fl.x < zone.maxX - 40) {
        let away = (fl.heading - entryHeading) % TAU;
        if (away > Math.PI) away -= TAU;
        if (away < -Math.PI) away += TAU;
        // Under seven degrees across the whole crossing: the only turning
        // left is the eased-out tail of whatever curve it entered on.
        expect(Math.abs(away)).toBeLessThan(0.12);
        expect(fl.speed).toBeGreaterThan(0);
      }
    }
  });

  it('flies a loop right round rather than merely curving', () => {
    const fl = new Flight({ bounds, cruise, random: rng(13) });
    // Parked in the middle so the loop has all the room it asks for.
    fl.x = width / 2;
    fl.y = height / 2;
    fl.loop();
    let net = 0;
    let peak = 0;
    let lastHeading = fl.heading;
    for (let i = 0; i < 60 * 9; i += 1) {
      fl.step(dt);
      let turned = (fl.heading - lastHeading) % TAU;
      if (turned > Math.PI) turned -= TAU;
      if (turned < -Math.PI) turned += TAU;
      net += turned;
      // The loop's summit: ordinary wandering never accumulates anywhere
      // near a full revolution in one direction before drifting back.
      peak = Math.max(peak, Math.abs(net));
      lastHeading = fl.heading;
    }
    expect(peak).toBeGreaterThan(5);
  });
});

/**
 * The component's safety promises, checked as source because the runtime
 * here has no DOM: the rAF loop must be cancellable, must gate itself on
 * `prefers-reduced-motion` in JS (the CSS animation squash cannot reach a
 * canvas), must stop off screen, and the markup must carry no `style`
 * attribute - the panel serves `style-src 'self'`, under which a browser
 * parses one and throws it away without a word.
 */
describe('NightRocket [Unit]', () => {
  const source = readFileSync(
    new URL('../src/components/NightRocket.svelte', import.meta.url),
    'utf8',
  );

  it('cancels its animation frame on teardown', () => {
    expect(source).toContain('cancelAnimationFrame');
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

  it('clears its launch timer on teardown', () => {
    expect(source).toContain('clearTimeout');
  });
});

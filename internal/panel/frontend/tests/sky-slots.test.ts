import { describe, expect, it } from 'vitest';

import { SkySlots } from '../src/lib/sky-slots';

/**
 * The seat budget behind the easter eggs' two-at-a-time rule. Small enough
 * to read in one breath; pinned because every flying thing on the page
 * stands on it, across both theme homes at once.
 */
describe('SkySlots [Unit]', () => {
  it('seats two and refuses a third', () => {
    const slots = new SkySlots();
    expect(slots.take()).toBe(true);
    expect(slots.take()).toBe(true);
    expect(slots.take()).toBe(false);
  });

  it('frees a seat on release', () => {
    const slots = new SkySlots();
    slots.take();
    slots.take();
    slots.release();
    expect(slots.take()).toBe(true);
    expect(slots.take()).toBe(false);
  });

  it('cannot go below empty and mint phantom seats', () => {
    const slots = new SkySlots();
    slots.release();
    slots.release();
    expect(slots.take()).toBe(true);
    expect(slots.take()).toBe(true);
    expect(slots.take()).toBe(false);
  });
});

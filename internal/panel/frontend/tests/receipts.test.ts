import { beforeEach, describe, expect, it, vi } from 'vitest';

import { receipts } from '#lib/receipts.svelte.js';

/**
 * The line: who is on screen, who waits, and what Undo does to both.
 */
describe('the receipts a change leaves behind [Unit]', () => {
  beforeEach(() => {
    receipts.clear();
  });

  it('says nothing until something happens', () => {
    expect(receipts.current).toBeNull();
  });

  it('replaces a timed receipt with whatever happened next', () => {
    receipts.say('Removed release-please');
    receipts.say('Added dependencies');

    expect(receipts.current?.say).toBe('Added dependencies');
  });

  it('lets a sticky receipt keep the floor, and queues the newcomer', () => {
    receipts.say('Pausing background work', { sticky: true });
    receipts.say('Saved - CI re-checks runs every 30 seconds');

    expect(receipts.current?.say).toBe('Pausing background work');

    receipts.dismiss();
    expect(receipts.current?.say).toBe('Saved - CI re-checks runs every 30 seconds');
  });

  it('takes the receipt off before the change is taken back', () => {
    const undo = vi.fn(() => {
      // The receipt is gone by the time the change is undone, so an undo that reports
      // what it did has the floor rather than being overwritten a moment later.
      expect(receipts.current).toBeNull();
    });
    receipts.say('Removed release-please', { undo });
    receipts.undo();

    expect(undo).toHaveBeenCalledOnce();
  });

  it('has nothing to undo where nothing was offered', () => {
    receipts.say('Saved - the schedule holds');
    receipts.undo();

    expect(receipts.current).toBeNull();
  });

  it('empties the line, waiting receipts and all', () => {
    receipts.say('Pausing background work', { sticky: true });
    receipts.say('Saved');
    receipts.clear();

    expect(receipts.current).toBeNull();
    receipts.dismiss();
    expect(receipts.current).toBeNull();
  });
});

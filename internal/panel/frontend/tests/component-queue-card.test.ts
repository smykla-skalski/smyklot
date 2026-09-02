// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import QueueList, { type QueueCard } from '../src/lib/components/QueueList.svelte';
import type { QueueItem } from '../src/lib/types.js';

const NOW = Date.parse('2026-08-24T12:00:00Z');

function work(id: string): QueueItem {
  return {
    id,
    kind: 'reaction_scan',
    lane: 'maintenance',
    title: 'Scan for new commands',
    summary: 'smykla-skalski/smyklot',
    state: 'scheduled',
    priority: 'normal',
    priority_overridden: false,
    window_mode: 'respect',
    immediate: false,
    not_before: '2026-08-24T13:00:00Z',
    eligible_at: '2026-08-24T13:00:00Z',
    work_ahead: 0,
    progress_current: 0,
    progress_total: 0,
    attempt: 0,
    revision: 1,
    created_at: '2026-08-24T12:00:00Z',
    updated_at: '2026-08-24T12:00:00Z',
  };
}

function card(patch: Partial<QueueCard> = {}): QueueCard {
  return {
    id: 'live',
    title: 'Running and waiting',
    items: [work('one')],
    count: 'Showing 1-1\u{a0}of 1',
    more: false,
    busy: false,
    onMore: vi.fn(),
    ...patch,
  };
}

/**
 * A card's foot is a promise that the card is holding something back.
 *
 * The count was drawn under every list, including the ones a reader can see the whole
 * of - and before that it counted the whole page from inside one card, so a card with
 * one row in it said "Showing 1-4 of 4".
 */
describe('a queue card [Component]', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })),
    );
  });

  it('says nothing about how much it holds when it holds all of it', () => {
    render(QueueList, {
      cards: [card()],
      clock: () => NOW,
      onOpen: vi.fn(),
      onAction: vi.fn(),
    });

    expect(screen.queryByText(/Showing/u)).toBeNull();
    expect(screen.queryByRole('button', { name: 'Show more' })).toBeNull();
  });

  it('counts its own rows and brings more of itself down', async () => {
    const onMore = vi.fn();
    render(QueueList, {
      cards: [
        card({
          items: [work('one'), work('two')],
          count: 'Showing 1-2\u{a0}of 9',
          more: true,
          onMore,
        }),
      ],
      clock: () => NOW,
      onOpen: vi.fn(),
      onAction: vi.fn(),
    });

    // The count holds a non-breaking space, which the query normalizes away.
    expect(screen.getByText(/Showing 1-2\s+of 9/u)).not.toBeNull();
    await fireEvent.click(screen.getByRole('button', { name: 'Show more' }));
    expect(onMore).toHaveBeenCalledTimes(1);
  });
});

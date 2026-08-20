// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import Rail, { workspaceHue, workspaceInitials } from '../src/lib/components/Rail.svelte';

class TestResizeObserver {
  observe(): void {}
  disconnect(): void {}
}

describe('Rail [Component]', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', TestResizeObserver);
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({
        matches: false,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      })),
    );
  });

  afterEach(() => vi.unstubAllGlobals());

  it('opens the Inbox from its real link and exposes the unread count', async () => {
    const onSelectInbox = vi.fn();

    render(Rail, {
      viewer: null,
      targets: [],
      selectedId: null,
      targetHref: () => '#',
      onSelectTarget: vi.fn(),
      rootMode: false,
      rootEnabled: false,
      rootEntryHref: '/root',
      onEnterRoot: vi.fn(),
      inboxHref: '/inbox',
      inboxActive: false,
      onSelectInbox,
      unreadCount: 3,
      theme: 'system',
      onSelectTheme: vi.fn(),
      onSignOut: vi.fn(),
    });

    const inbox = screen.getByRole('link', { name: 'Inbox - 3 unread' });
    expect(inbox.getAttribute('href')).toBe('/inbox');

    await fireEvent.click(inbox);

    expect(onSelectInbox).toHaveBeenCalledOnce();
  });
});

describe('the workspace identity', () => {
  it('reads initials from up to two words of the name', () => {
    expect(workspaceInitials('Smykla Skalski')).toBe('SS');
    expect(workspaceInitials('bartsmykla')).toBe('B');
    expect(workspaceInitials('Oak & Pine')).toBe('OP');
    expect(workspaceInitials('Vantage Labs International')).toBe('VL');
    expect(workspaceInitials('')).toBe('?');
  });

  it('hashes a login to a stable hue inside the wheel', () => {
    const hue = workspaceHue('smykla-skalski');
    expect(hue).toBe(workspaceHue('smykla-skalski'));
    expect(hue).toBeGreaterThanOrEqual(0);
    expect(hue).toBeLessThan(360);
    // Different logins should not all land on one colour.
    const hues = new Set(
      ['smykla-skalski', 'bartsmykla', 'acme', 'northwind', 'zephyr'].map(workspaceHue),
    );
    expect(hues.size).toBeGreaterThan(3);
  });
});

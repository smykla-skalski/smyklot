// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import IdentityBar from '../src/lib/components/IdentityBar.svelte';

class TestResizeObserver {
  observe(): void {}
  disconnect(): void {}
}

describe('IdentityBar [Component]', () => {
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

    render(IdentityBar, {
      viewer: null,
      targets: [],
      selectedId: null,
      targetHref: () => '#',
      onSelectTarget: vi.fn(),
      onSignOut: vi.fn(),
      view: 'repositories',
      viewHref: (view) => `/i/acme/${view}`,
      onSelectView: vi.fn(),
      showUsers: true,
      showViews: true,
      showNavigation: true,
      collapsed: false,
      onToggleCollapsed: vi.fn(),
      theme: 'system',
      onSelectTheme: vi.fn(),
      rootMode: false,
      rootValue: 'overview',
      rootHrefFor: (section) => `/root/${section}`,
      onSelectRoot: vi.fn(),
      rootEntryHref: '/root',
      onEnterRoot: vi.fn(),
      returnHref: '/i/acme/repositories',
      onReturnToPanel: vi.fn(),
      inboxHref: '/inbox',
      inboxActive: false,
      onSelectInbox,
      unreadCount: 3,
    });

    const inbox = screen.getByRole('link', { name: 'Inbox, 3 unread' });
    expect(inbox.getAttribute('href')).toBe('/inbox');

    await fireEvent.click(inbox);

    expect(onSelectInbox).toHaveBeenCalledOnce();
  });
});

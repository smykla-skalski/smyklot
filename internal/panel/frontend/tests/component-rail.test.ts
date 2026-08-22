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

describe('the rail identity tiles', () => {
  beforeEach(() => {
    // The account menu's Popover portals into the shell.
    document.body.innerHTML = '<main class="app-shell"></main>';
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

  const account = (login: string, avatarUrl: string | null) => ({
    id: `acct-${login}`,
    provider: 'github',
    subject_id: login,
    login,
    display_name: login,
    avatar_url: avatarUrl,
  });

  const target = (login: string, avatarUrl: string | null) =>
    ({
      id: `ws-${login}`,
      account: account(login, avatarUrl),
    }) as never;

  const props = {
    selectedId: null,
    targetHref: () => '#',
    onSelectTarget: vi.fn(),
    rootMode: false,
    rootEnabled: false,
    rootEntryHref: '/root',
    onEnterRoot: vi.fn(),
    inboxHref: '/inbox',
    inboxActive: false,
    onSelectInbox: vi.fn(),
    unreadCount: 0,
    theme: 'system',
    onSelectTheme: vi.fn(),
    onSignOut: vi.fn(),
  } as const;

  // The shell redesign replaced IdentityBar's real avatars with generated
  // marks unconditionally, which lost every profile picture in production.
  // The mark is the fallback, never the identity.
  it('shows the real avatar on a workspace tile and on the user tile', () => {
    const { container } = render(Rail, {
      ...props,
      viewer: {
        account: account('bartsmykla', 'https://avatars.example/u/1'),
        system_role: 'none',
        status: 'active',
        target_count: 1,
      } as never,
      targets: [target('smykla-skalski', 'https://avatars.example/o/2')],
    });

    const sources = [...container.querySelectorAll('img.avatar')].map((img) =>
      img.getAttribute('src'),
    );
    expect(sources).toContain('https://avatars.example/o/2');
    expect(sources).toContain('https://avatars.example/u/1');
  });

  it('falls back to the generated mark when the account has no picture', () => {
    const { container } = render(Rail, {
      ...props,
      viewer: null,
      targets: [target('smykla-skalski', null)],
    });

    expect(container.querySelector('img.avatar')).toBeNull();
    expect(container.querySelector('.rail-ws .t')?.textContent).toBe('SS');
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

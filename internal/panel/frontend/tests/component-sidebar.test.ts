// @vitest-environment jsdom
import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import Sidebar, { type SidebarPage } from '../src/lib/components/Sidebar.svelte';

class TestResizeObserver {
  observe(): void {}
  disconnect(): void {}
}

const cleanPage: SidebarPage = {
  id: 'repositories',
  label: 'Repositories',
  icon: 'repositories',
  href: '/repositories',
  active: false,
};

function syncPage(active: boolean): SidebarPage {
  return {
    id: 'sync',
    label: 'Sync',
    icon: 'refresh',
    href: '/sync',
    active,
    kids: [
      { id: 'overview', label: 'Overview', href: '/sync', active: active },
      { id: 'settings', label: 'Settings', href: '/sync/settings', active: false, dirty: true },
      { id: 'plan', label: 'Plan', href: '/sync/plan', active: false, count: 2, signal: true },
    ],
  };
}

function mount(pages: SidebarPage[], collapsed = false) {
  return render(Sidebar, {
    kicker: 'Workspace',
    title: 'Acme',
    pages,
    collapsed,
    onToggleCollapsed: vi.fn(),
    onSelectPage: vi.fn(),
    onSelectKid: vi.fn(),
  });
}

describe('Sidebar dirty state [Component]', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', TestResizeObserver);
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() })),
    );
  });

  afterEach(() => vi.unstubAllGlobals());

  it('keeps an expanded child state on the precise leaf and separate from plan signal', () => {
    const { container } = mount([syncPage(true), cleanPage]);

    expect(
      screen
        .getByRole('link', { name: 'Settings Unsaved changes' })
        .classList.contains('has-dirty'),
    ).toBe(true);
    expect(screen.getByRole('link', { name: 'Sync' }).classList.contains('has-dirty')).toBe(false);
    expect(container.querySelector('.tree-page.has-signal')).not.toBeNull();
    expect(container.querySelector('.tree-page.has-dirty')).toBeNull();
    expect(screen.getByRole('link', { name: 'Plan 2' }).classList.contains('has-dirty')).toBe(
      false,
    );
  });

  it('bubbles a dirty child to an inactive page whose children are hidden', () => {
    mount([syncPage(false), { ...cleanPage, active: true }]);

    const sync = screen.getByRole('link', { name: 'Sync Unsaved changes' });
    expect(sync.closest('.tree-page')?.classList.contains('has-dirty')).toBe(true);
    expect(sync.querySelector('.dirty-mark')?.textContent).toBe('*');
  });

  it('bubbles a dirty child to its page in the collapsed strip', () => {
    mount([syncPage(true), cleanPage], true);

    const sync = screen.getByRole('link', { name: 'Sync Unsaved changes' });
    expect(sync.closest('.tree-page')?.classList.contains('has-dirty')).toBe(true);
    expect(sync.querySelector('.dirty-mark')?.getAttribute('aria-hidden')).toBe('true');
  });

  it('marks a dirty page with no children directly', () => {
    mount([{ ...cleanPage, active: true, dirty: true }]);

    expect(
      screen
        .getByRole('link', { name: 'Repositories Unsaved changes' })
        .classList.contains('has-dirty'),
    ).toBe(true);
  });
});

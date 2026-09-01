// @vitest-environment jsdom
import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import Sidebar, { type SidebarEntry } from '../src/lib/components/Sidebar.svelte';

class TestResizeObserver {
  observe(): void {}
  disconnect(): void {}
}

const ENTRIES: SidebarEntry[] = [
  {
    id: 'repositories',
    label: 'Repositories',
    icon: 'book',
    href: '/repositories',
    active: false,
  },
  { kind: 'group', id: 'group-sync', label: 'Sync' },
  {
    id: 'sync-overview',
    label: 'Sync status',
    icon: 'refresh',
    href: '/sync',
    active: true,
  },
  {
    id: 'sync-settings',
    label: 'Repository options',
    icon: 'sliders',
    href: '/sync/settings',
    active: false,
    dirty: true,
  },
  {
    id: 'sync-plan',
    label: 'Plan',
    icon: 'plan',
    href: '/sync/plan',
    active: false,
    count: 2,
    signal: true,
  },
  {
    id: 'settings',
    label: 'Workspace settings',
    icon: 'gear',
    href: '/settings',
    active: false,
    foot: true,
  },
];

function mount(entries: SidebarEntry[] = ENTRIES, collapsed = false) {
  return render(Sidebar, {
    kicker: 'Workspace',
    title: 'Acme',
    entries,
    collapsed,
    onToggleCollapsed: vi.fn(),
    onSelectRow: vi.fn(),
  });
}

describe('Sidebar tree [Component]', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', TestResizeObserver);
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() })),
    );
  });

  afterEach(() => vi.unstubAllGlobals());

  it('marks the row that holds unsaved configuration, and only that row', () => {
    mount();

    expect(
      screen
        .getByRole('link', { name: 'Repository options Unsaved changes' })
        .classList.contains('has-dirty'),
    ).toBe(true);
    expect(screen.getByRole('link', { name: 'Sync status' }).classList.contains('has-dirty')).toBe(
      false,
    );
    expect(screen.getByRole('link', { name: 'Plan 2' }).classList.contains('has-dirty')).toBe(
      false,
    );
  });

  it('speaks a waiting count as a signal beside its row', () => {
    const { container } = mount();

    const plan = screen.getByRole('link', { name: 'Plan 2' });
    expect(plan.querySelector('.tab-count.is-signal')?.textContent).toBe('2');
    expect(container.querySelectorAll('.tab-count.is-signal')).toHaveLength(1);
  });

  it('renders a heading as a label, never as a destination', () => {
    const { container } = mount();

    const headings = [...container.querySelectorAll('.tree-group')].map((node) => node.textContent);
    expect(headings).toEqual(['Sync']);
    expect(screen.queryByRole('link', { name: 'Sync' })).toBeNull();
  });

  it('stands the workspace settings row apart from the groups above it', () => {
    mount();

    expect(
      screen.getByRole('link', { name: 'Workspace settings' }).classList.contains('is-foot'),
    ).toBe(true);
  });

  it('keeps the mark on its own row in the collapsed strip', () => {
    mount(ENTRIES, true);

    const options = screen.getByRole('link', { name: 'Repository options Unsaved changes' });
    expect(options.querySelector('.dirty-mark')?.getAttribute('aria-hidden')).toBe('true');
    expect(options.querySelector('.dirty-mark')?.textContent).toBe('*');
  });
});

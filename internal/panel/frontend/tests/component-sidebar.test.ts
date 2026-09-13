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

function mount(
  entries: SidebarEntry[] = ENTRIES,
  collapsed = false,
  build = { version: '1.56.0' as string | null, serviceHost: 'smyklot.com' as string | null },
) {
  return render(Sidebar, {
    kicker: 'Workspace',
    title: 'Acme',
    entries,
    collapsed,
    onToggleCollapsed: vi.fn(),
    onSelectRow: vi.fn(),
    build,
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

  it('places build information after navigation without a page footer landmark', () => {
    const { container } = mount();
    const information = screen.getByRole('group', { name: 'Build information' });
    expect(information.closest('aside')).toBe(screen.getByRole('complementary'));
    expect(
      information.compareDocumentPosition(screen.getByRole('navigation')) &
        Node.DOCUMENT_POSITION_PRECEDING,
    ).toBeTruthy();
    expect(information.textContent).toMatch(/Panel\s*1\.56\.0/u);
    expect(information.textContent).toMatch(/Service\s*smyklot\.com/u);
    expect(container.querySelector('footer')).toBeNull();
  });

  it.each([
    { version: null, serviceHost: null },
    { version: '1.56.0', serviceHost: null },
    { version: null, serviceHost: 'service.example.com' },
  ])('renders only known build information: %j', (build) => {
    const { container } = mount(ENTRIES, false, build);
    const values = [...container.querySelectorAll('.build-info dd')].map(
      (node) => node.textContent,
    );
    expect(values).toEqual([build.version, build.serviceHost].filter((value) => value !== null));
    expect(container.querySelector('.sidebar-build') !== null).toBe(values.length > 0);
  });

  it('keeps long build values available as exact accessible text', () => {
    const build = { version: 'release-' + 'a'.repeat(160), serviceHost: 'host-' + 'b'.repeat(160) };
    const { container } = mount(ENTRIES, false, build);
    const values = [...container.querySelectorAll('.build-info dd')];
    expect(values.map((node) => node.textContent)).toEqual([build.version, build.serviceHost]);
    expect(values.map((node) => node.getAttribute('title'))).toEqual([
      build.version,
      build.serviceHost,
    ]);
  });

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

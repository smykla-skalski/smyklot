// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import SyncLabelsPage from '../src/lib/components/SyncLabelsPage.svelte';
import type { LabelsSaveInput } from '../src/lib/components/SyncLabelsPage.svelte';
import type { SyncConfig } from '../src/lib/types';

class TestResizeObserver {
  observe(): void {}
  disconnect(): void {}
}

function config(over: Partial<SyncConfig> = {}): SyncConfig {
  return {
    kind: 'labels',
    enabled: false,
    labels: [{ name: 'bug', color: 'd73a4a' }],
    allow_removal: false,
    excludes: [],
    revision: 1,
    updated_by: 'bart',
    updated_at: new Date(0).toISOString(),
    digest: '',
    document: {},
    unreadable: false,
    unavailable: '',
    ...over,
  };
}

describe('SyncLabelsPage [Component]', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', TestResizeObserver);
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  afterEach(() => vi.unstubAllGlobals());

  it('stages each edit immediately against the latest local draft', async () => {
    const sent: LabelsSaveInput[] = [];
    const controls: string[] = [];
    const onChange = vi.fn((wanted: LabelsSaveInput, controlId: string) => {
      sent.push(wanted);
      controls.push(controlId);
      return true;
    });

    render(SyncLabelsPage, {
      config: config(),
      readOnly: false,
      problem: null,
      sectionHref: () => '#',
      onOpenSection: vi.fn(),
      onChange,
    });

    await fireEvent.click(screen.getByRole('checkbox', { name: 'Label sync' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Remove bug' }));

    expect(sent).toHaveLength(2);
    expect(sent[1]).toMatchObject({ enabled: true, labels: [] });
    expect(controls).toEqual(['sync.labels.enabled', 'sync.labels.labels']);
  });

  it('keeps the local draft while the parent refreshes its saved base', async () => {
    const sent: LabelsSaveInput[] = [];
    const onChange = vi.fn((wanted: LabelsSaveInput) => {
      sent.push(wanted);
      return true;
    });
    const props = {
      config: config(),
      readOnly: false,
      problem: null,
      sectionHref: () => '#',
      onOpenSection: vi.fn(),
      onChange,
    };
    const page = render(SyncLabelsPage, props);

    await fireEvent.click(screen.getByRole('checkbox', { name: 'Label sync' }));
    await fireEvent.click(
      screen.getByRole('checkbox', { name: 'Remove labels this list does not name' }),
    );

    // The parent overlays the registry draft while its canonical revision
    // advances, so the component follows that overlaid value rather than the
    // newly fetched base.
    await page.rerender({
      ...props,
      config: config({ enabled: true, allow_removal: true, revision: 2 }),
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Remove bug' }));

    expect(sent[2]).toMatchObject({ enabled: true, allow_removal: true, labels: [] });
  });

  it('keeps an active editor open when the parent echoes its draft', async () => {
    const props = {
      config: config(),
      readOnly: false,
      problem: null,
      sectionHref: () => '#',
      onOpenSection: vi.fn(),
      onChange: vi.fn(() => true),
    };
    const page = render(SyncLabelsPage, props);

    await fireEvent.click(screen.getByRole('button', { name: 'Add a label' }));
    await fireEvent.input(screen.getByRole('textbox', { name: 'Label name' }), {
      target: { value: 'release' },
    });
    await page.rerender({
      ...props,
      config: config({
        labels: [
          { name: 'release', color: '0e8a16' },
          { name: 'bug', color: 'd73a4a' },
        ],
        revision: 2,
      }),
    });

    expect((screen.getByRole('textbox', { name: 'Label name' }) as HTMLInputElement).value).toBe(
      'release',
    );
  });
});

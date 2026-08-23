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
    const onSave = vi.fn((wanted: LabelsSaveInput) => {
      sent.push(wanted);
      return Promise.resolve(true);
    });

    render(SyncLabelsPage, {
      config: config(),
      readOnly: false,
      problem: null,
      sectionHref: () => '#',
      onOpenSection: vi.fn(),
      onSave,
    });

    await fireEvent.click(screen.getByRole('checkbox', { name: 'Label sync' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Remove bug' }));

    expect(sent).toHaveLength(2);
    expect(sent[1]).toMatchObject({ enabled: true, labels: [] });
  });

  it('keeps the local draft while the parent refreshes its saved base', async () => {
    const sent: LabelsSaveInput[] = [];
    const onSave = vi.fn((wanted: LabelsSaveInput) => {
      sent.push(wanted);
      return Promise.resolve(true);
    });
    const props = {
      config: config(),
      readOnly: false,
      problem: null,
      sectionHref: () => '#',
      onOpenSection: vi.fn(),
      onSave,
    };
    const page = render(SyncLabelsPage, props);

    await fireEvent.click(screen.getByRole('checkbox', { name: 'Label sync' }));
    await fireEvent.click(
      screen.getByRole('checkbox', { name: 'Remove labels this list does not name' }),
    );

    // A background read may advance the saved base, but it must not replace the
    // installation draft currently being edited.
    await page.rerender({
      ...props,
      config: config({ enabled: true, allow_removal: false, revision: 2 }),
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Remove bug' }));

    expect(sent[2]).toMatchObject({ enabled: true, allow_removal: true, labels: [] });
  });
});

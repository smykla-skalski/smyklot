// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import SyncLabelsPage from '../src/lib/components/SyncLabelsPage.svelte';
import {
  createLatestLabelsSave,
  type LabelsSaveInput,
} from '../src/lib/components/SyncLabelsPage.svelte';
import type { SyncConfig } from '../src/lib/types';

function input(name: string): LabelsSaveInput {
  return {
    enabled: true,
    labels: [{ name, color: '0e8a16' }],
    allow_removal: false,
    excludes: [],
  };
}

describe('the labels save queue [Unit]', () => {
  it('sends the latest edit only after the prior revision lands', async () => {
    let releaseFirst!: () => void;
    const firstLanded = new Promise<void>((resolve) => (releaseFirst = resolve));
    const sent: LabelsSaveInput[] = [];
    const save = vi.fn(async (wanted: LabelsSaveInput) => {
      sent.push(wanted);
      if (sent.length === 1) await firstLanded;
      return true;
    });
    const receipt = vi.fn();
    const enqueue = createLatestLabelsSave(save, receipt);

    enqueue(input('first'));
    enqueue(input('second'));
    enqueue(input('latest'));

    expect(sent.map((held) => held.labels[0]?.name)).toEqual(['first']);
    releaseFirst();
    await vi.waitFor(() => expect(save).toHaveBeenCalledTimes(2));

    expect(sent.map((held) => held.labels[0]?.name)).toEqual(['first', 'latest']);
    expect(receipt).toHaveBeenCalledTimes(2);
  });

  it('tries the newest waiting edit after the prior request fails', async () => {
    let releaseFirst!: () => void;
    const firstFinished = new Promise<void>((resolve) => (releaseFirst = resolve));
    const sent: LabelsSaveInput[] = [];
    const save = vi.fn(async (wanted: LabelsSaveInput) => {
      sent.push(wanted);
      const attempt = sent.length;
      if (attempt === 1) await firstFinished;
      return attempt > 1;
    });
    const receipt = vi.fn();
    const enqueue = createLatestLabelsSave(save, receipt);

    enqueue(input('first'));
    enqueue(input('latest'));
    releaseFirst();
    await vi.waitFor(() => expect(save).toHaveBeenCalledTimes(2));

    expect(sent.map((held) => held.labels[0]?.name)).toEqual(['first', 'latest']);
    expect(receipt).toHaveBeenCalledOnce();
  });
});

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

  it('keeps an optimistic toggle in a label edit queued behind it', async () => {
    let releaseFirst!: (landed: boolean) => void;
    const first = new Promise<boolean>((resolve) => (releaseFirst = resolve));
    const sent: LabelsSaveInput[] = [];
    const onSave = vi.fn((wanted: LabelsSaveInput) => {
      sent.push(wanted);
      return sent.length === 1 ? first : Promise.resolve(true);
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
    expect(sent).toHaveLength(1);

    releaseFirst(true);
    await vi.waitFor(() => expect(onSave).toHaveBeenCalledTimes(2));

    expect(sent[1]).toMatchObject({ enabled: true, labels: [] });
  });

  it('keeps the newest draft while a prior response refreshes config', async () => {
    let releaseFirst!: (landed: boolean) => void;
    const first = new Promise<boolean>((resolve) => (releaseFirst = resolve));
    const sent: LabelsSaveInput[] = [];
    const onSave = vi.fn((wanted: LabelsSaveInput) => {
      sent.push(wanted);
      return sent.length === 1 ? first : Promise.resolve(true);
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
    expect(sent).toHaveLength(1);

    // The PUT has landed and advanced the parent's revision, but its plan
    // refresh is still holding the first onSave promise open.
    await page.rerender({
      ...props,
      config: config({ enabled: true, allow_removal: false, revision: 2 }),
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Remove bug' }));
    releaseFirst(true);
    await vi.waitFor(() => expect(onSave).toHaveBeenCalledTimes(2));

    expect(sent[1]).toMatchObject({ enabled: true, allow_removal: true, labels: [] });
  });
});

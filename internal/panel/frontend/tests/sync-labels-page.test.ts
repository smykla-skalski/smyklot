import { describe, expect, it, vi } from 'vitest';

import {
  createLatestLabelsSave,
  type LabelsSaveInput,
} from '../src/lib/components/SyncLabelsPage.svelte';

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
});

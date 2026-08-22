import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import { fixtureApi } from '../stories/support/api';
import {
  SYNC_FILES_CONTEXT,
  SYNC_STATUS,
  SYNC_STATUS_IN_STEP,
  TARGET,
} from '../stories/support/fixtures';

const syncViewStory = readFileSync(
  new URL('../stories/views/SyncView.stories.svelte', import.meta.url),
  'utf8',
);

function storyBody(name: string): string {
  const escaped = name.replace(/[.*+?^${}()|[\]\\]/gu, '\\$&');
  const found = syncViewStory.match(
    new RegExp(`<Story name="${escaped}">([\\s\\S]*?)</Story>`, 'u'),
  );
  expect(found, `${name} story was not found`).not.toBeNull();
  return found?.[1] ?? '';
}

describe('catalogue fixtures [Unit]', () => {
  it('answers every independent read needed to open the sync view', async () => {
    const api = fixtureApi();
    const [status, context] = await Promise.all([
      api.fetchSyncStatus(TARGET.id),
      api.fetchSyncFilesContext(TARGET.id),
    ]);

    expect(status).toEqual(SYNC_STATUS);
    expect(context).toEqual(SYNC_FILES_CONTEXT);
    expect(context.repositories).toBe(status.repositories.length);
    expect(context.covered).toBe(
      status.repositories.filter((row) => row.cells.files.state !== 'off').length,
    );
  });

  it('keeps the settled sync story settled across the whole fleet', () => {
    expect(SYNC_STATUS_IN_STEP.repositories.length).toBeGreaterThan(0);
    const states = SYNC_STATUS_IN_STEP.repositories.flatMap((row) =>
      Object.values(row.cells).map((cell) => cell.state),
    );
    expect(new Set(states)).toEqual(new Set(['in_step']));
    expect(storyBody('Already in step')).toContain('fetchStatus={async () => SYNC_STATUS_IN_STEP}');
  });

  it('opens the unreadable story on a surface that displays the warning', () => {
    const unreadable = storyBody('Unreadable');
    expect(unreadable).toContain('section="labels"');
    expect(unreadable).toContain('unreadable: true');
  });
});

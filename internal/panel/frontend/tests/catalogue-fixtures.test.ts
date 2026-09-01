import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import { fixtureApi } from '../stories/support/api';
import {
  AUDIT,
  FAILURES,
  INVITATIONS,
  REPOSITORIES,
  ROOT_WORKSPACE,
  ROOT_TARGET,
  SYNC_CONFIGS,
  SYNC_FILES_CONTEXT,
  SYNC_PLAN,
  SYNC_STATUS,
  SYNC_STATUS_IN_STEP,
  TARGET,
  USERS,
} from '../stories/support/fixtures';

const syncViewStory = readFileSync(
  new URL('../stories/views/SyncView.stories.svelte', import.meta.url),
  'utf8',
);
const workspaceView = readFileSync(
  new URL('../src/lib/components/WorkspaceView.svelte', import.meta.url),
  'utf8',
);
const workspaceViewStory = readFileSync(
  new URL('../stories/views/WorkspaceView.stories.svelte', import.meta.url),
  'utf8',
);
const preview = readFileSync(new URL('../.storybook/preview.ts', import.meta.url), 'utf8');

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

  it('answers every read needed to open the root workspace views', async () => {
    const api = fixtureApi({ fetchRootTargetSettings: async () => ROOT_TARGET });
    const [target, repositories, users, invitations, audit, failures] = await Promise.all([
      api.fetchRootTargetSettings(TARGET.id),
      api.fetchRootRepositories(TARGET.id, {
        query: '',
        sort: 'name_asc',
        limit: 20,
        state: 'all',
        files: [],
        setting: { mode: 'all' },
      }),
      api.fetchRootTargetUsers(TARGET.id, {
        query: '',
        sort: 'name_asc',
        limit: 20,
        roles: [],
        statuses: [],
      }),
      api.fetchRootTargetInvitations(TARGET.id, {
        query: '',
        sort: 'created_newest',
        limit: 20,
        roles: [],
        statuses: [],
      }),
      api.fetchRootTargetAudit(TARGET.id, { query: '', sort: 'newest', limit: 20 }),
      api.fetchRootTargetFailures(TARGET.id, {
        query: '',
        sort: 'newest',
        limit: 20,
        kind: 'all',
      }),
    ]);

    expect(target).toEqual(ROOT_TARGET);
    expect(ROOT_WORKSPACE).toMatchObject({
      id: target.id,
      installation_id: target.installation_id,
      type: target.type,
      account: target.account,
      repository_counts: target.repository_counts,
      owned_by_viewer: false,
    });
    expect(target.access_source).toBe('root');
    expect(target.capabilities).toEqual({ read: true, write: false, manage_target_users: false });
    expect(repositories.items).toHaveLength(REPOSITORIES.length);
    expect(users.items).toHaveLength(USERS.length);
    expect(invitations.items).toHaveLength(INVITATIONS.length);
    expect(audit.items).toHaveLength(AUDIT.length);
    expect(failures.items).toHaveLength(FAILURES.length);
    await expect(api.fetchRootElevation(TARGET.id)).rejects.toMatchObject({
      status: 404,
      code: 'not_found',
    });
  });

  it('freezes every catalogue story at the fixture instant', () => {
    expect(preview).toContain("import { NOW } from '../stories/support/fixtures.js'");
    expect(preview).toContain('Date.now = () => NOW');
    expect(preview).toContain('Date.now = liveNow');
  });

  it('keeps the settled sync story settled across the whole fleet', () => {
    expect(SYNC_STATUS_IN_STEP.repositories.length).toBeGreaterThan(0);
    const states = SYNC_STATUS_IN_STEP.repositories.flatMap((row) =>
      Object.values(row.cells).map((cell) => cell.state),
    );
    expect(new Set(states)).toEqual(new Set(['in_step']));
    expect(storyBody('Already in step')).toContain('fetchStatus={async () => SYNC_STATUS_IN_STEP}');
  });

  it('keeps the plan story coherent with the fleet status', () => {
    expect(SYNC_PLAN).not.toBeNull();
    if (SYNC_PLAN === null) return;
    const planned = Object.values(SYNC_PLAN.counts).reduce((total, count) => total + count, 0);
    const reported = SYNC_STATUS.repositories.reduce(
      (total, row) =>
        total +
        Object.values(row.cells).reduce((rowTotal, cell) => rowTotal + (cell.changes ?? 0), 0),
      0,
    );

    expect(SYNC_PLAN.actions).toHaveLength(planned);
    expect(reported).toBe(planned);
    expect(syncViewStory).toContain('fetchPlan: async () => ({ plan: PLAN })');
    expect(syncViewStory).toContain('SYNC_CONFIGS.get(`${TARGET.id}/${kind}`)');
    expect(syncViewStory).toContain('fetchFilesContext: async () => SYNC_FILES_CONTEXT');
    expect(syncViewStory).toContain('clock: () => NOW');

    const desiredLabels = new Set(
      (SYNC_CONFIGS.get(`${TARGET.id}/labels`)?.labels ?? []).map((label) => label.name),
    );
    const plannedLabels = SYNC_PLAN.actions
      .filter((action) => action.kind === 'labels' && action.operation !== 'delete')
      .map((action) => action.subject);
    expect(plannedLabels.length).toBeGreaterThan(0);
    expect(plannedLabels.every((label) => desiredLabels.has(label))).toBe(true);
  });

  it('keeps the workspace wrapper on the fixture clock', () => {
    expect(workspaceViewStory).toContain('clock: () => NOW');
    expect(workspaceView).toMatch(/<SyncView[\s\S]*?\n\s+\{clock\}\n\s+\/>/u);
  });

  it('opens the unreadable story on a surface that displays the warning', () => {
    const unreadable = storyBody('Unreadable');
    expect(unreadable).toContain('section="labels"');
    expect(unreadable).toContain('unreadable: true');
  });
});

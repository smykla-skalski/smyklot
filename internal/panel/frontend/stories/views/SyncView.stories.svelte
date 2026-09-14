<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import { seed, VIEWER } from '../../dev/fixtures';
  import { mockSyncRunNow } from '../../dev/sync-run-now';
  import { mockSyncHistory, mockSyncHistoryPage } from '../../dev/sync-history';
  import SyncView from '#lib/components/SyncView.svelte';
  import Seeded from '../support/Seeded.svelte';
  import {
    emptySyncConfig,
    NOW,
    SYNC_CONFIGS,
    SYNC_FILES_CONTEXT,
    SYNC_PLAN,
    SYNC_STATUS,
    SYNC_STATUS_IN_STEP,
    TARGET,
  } from '../support/fixtures.js';
  import type { SyncConfig, SyncRunNowInput } from '#lib/types.js';

  /* Plan, status and desired documents all come from one mock seed. A story that
     restates any one of them can describe changes its own editors do not request. */
  const config = (kind: string, over: Partial<SyncConfig> = {}): SyncConfig => ({
    ...(SYNC_CONFIGS.get(`${TARGET.id}/${kind}`) ?? emptySyncConfig(kind)),
    ...over,
  });

  const PLAN = SYNC_PLAN;
  if (PLAN === null) throw new Error('the catalogue seed must include a sync plan');
  const base = {
    targetId: TARGET.id,
    actorId: VIEWER.id,
    section: 'overview' as const,
    readOnly: false,
    clock: () => NOW,
    fetchConfig: async (_id: string, kind: string) => config(kind),
    fetchPlan: async () => ({ plan: PLAN }),
    fetchHistory: async (id: string, request: { limit: number; cursor?: string }) =>
      mockSyncHistoryPage(
        mockSyncHistory(seed(undefined, NOW), id),
        request.limit,
        request.cursor ?? null,
      ),
    checkHref: (id: string) => `/sync/check/${id}`,
    fetchCheck: async () => {
      throw new Error('No check selected');
    },
    checkResultHref: (checkId: string, planId: string) => `/sync/check/${checkId}/result/${planId}`,
    onOpenCheck: () => {},
    historyResultHref: (id: string) => `#/sync/history/${id}`,
    onOpenHistoryResult: () => {},
    approvePlan: async () => ({ plan: { ...PLAN, state: 'approved' as const } }),
    discardPlan: async () => {},
    runSyncNow: async (id: string, input: SyncRunNowInput) => {
      const reply = mockSyncRunNow(seed(undefined, NOW), id, input, NOW);
      if ('message' in reply.body) throw new Error(reply.body.message);
      return reply.body;
    },
    canControl: true,
    fetchStatus: async () => SYNC_STATUS,
    sectionHref: (section: string) => `#/sync/${section}`,
    onOpenSection: () => {},
    rulesetHref: (name: string) => `#/sync/rulesets/${name}`,
    onOpenRuleset: () => {},
    fileHref: (path: string) => `#/sync/files/${path}`,
    onOpenFile: () => {},
    fetchFilesContext: async () => SYNC_FILES_CONTEXT,
    fetchOverride: async () => {
      throw new Error('not in this story');
    },
  };

  const { Story } = defineMeta({ title: 'Views/SyncView', component: SyncView, args: base });
</script>

<!--
  What every repository in a workspace should share, and what would change if it
  were applied. Nothing happens on GitHub until a plan is approved - the plan is the
  whole point of the screen, and it is why the labels editor and the Apply button are
  never the same press.
-->
<Story name="With a plan">
  {#snippet template(args)}
    <Seeded><SyncView {...args} /></Seeded>
  {/snippet}
</Story>

<!-- Nothing would change, so there is nothing to approve. -->
<Story name="Already in step">
  {#snippet template(args)}
    <Seeded>
      <SyncView
        {...args}
        fetchPlan={async () => ({ plan: null })}
        fetchStatus={async () => SYNC_STATUS_IN_STEP}
      />
    </Seeded>
  {/snippet}
</Story>

<!-- A viewer may read the configuration and approve nothing. -->
<Story name="Read only">
  {#snippet template(args)}
    <Seeded><SyncView {...args} readOnly /></Seeded>
  {/snippet}
</Story>

<!-- The file on GitHub cannot be parsed, so no plan can be trusted. -->
<Story name="Unreadable">
  {#snippet template(args)}
    <Seeded>
      <SyncView
        {...args}
        section="labels"
        fetchConfig={async (_id, kind) => config(kind, { unreadable: true })}
        fetchPlan={async () => ({ plan: null })}
      />
    </Seeded>
  {/snippet}
</Story>

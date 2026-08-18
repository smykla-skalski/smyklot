<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import SyncView from '#lib/components/SyncView.svelte';
  import Seeded from '../support/Seeded.svelte';
  import { NOW } from '../support/fixtures.js';
  import type { SyncConfig, SyncPlan } from '#lib/types.js';

  const at = (offsetMs: number) => new Date(NOW + offsetMs).toISOString();

  const config = (kind: string, over: Partial<SyncConfig> = {}): SyncConfig => ({
    kind,
    enabled: true,
    labels: [
      { name: 'bug', color: 'd73a4a', description: 'Something is broken' },
      { name: 'chore', color: 'cfd3d7' },
    ],
    allow_removal: false,
    excludes: [],
    revision: 4,
    updated_by: 'bart',
    updated_at: at(-2 * 60 * 60_000),
    digest: 'sha256:9f2c',
    document: {},
    unreadable: false,
    unavailable: '',
    ...over,
  });

  const PLAN: SyncPlan = {
    id: 'plan-1',
    trigger: 'manual',
    state: 'computed',
    digest: 'sha256:9f2c',
    counts: { create: 2, update: 1, delete: 0 },
    actions: [
      {
        repository: 'smyklot',
        kind: 'labels',
        operation: 'create',
        subject: 'bug',
        state: 'pending',
      },
      {
        repository: 'platform-infra',
        kind: 'labels',
        operation: 'update',
        subject: 'chore',
        before: 'ededed',
        after: 'cfd3d7',
        state: 'pending',
      },
    ],
    computed_at: at(-5 * 60_000),
    expires_at: at(55 * 60_000),
  };

  const base = {
    targetId: '2001',
    readOnly: false,
    fetchConfig: async (_id: string, kind: string) => config(kind),
    saveConfig: async (_id: string, kind: string) => config(kind),
    fetchPlan: async () => ({ plan: PLAN }),
    approvePlan: async () => ({ plan: { ...PLAN, state: 'approved' as const } }),
  };

  const { Story } = defineMeta({ title: 'Views/SyncView', component: SyncView, args: base });
</script>

<!--
  What every repository in an installation should share, and what would change if it
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
      <SyncView {...args} fetchPlan={async () => ({ plan: null })} />
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
        fetchConfig={async (_id, kind) => config(kind, { unreadable: true })}
        fetchPlan={async () => ({ plan: null })}
      />
    </Seeded>
  {/snippet}
</Story>

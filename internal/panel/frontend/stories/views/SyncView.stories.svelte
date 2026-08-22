<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import SyncView from '#lib/components/SyncView.svelte';
  import Seeded from '../support/Seeded.svelte';
  import { NOW } from '../support/fixtures.js';
  import type { SyncConfig, SyncPlan } from '#lib/types.js';

  const at = (offsetMs: number) => new Date(NOW + offsetMs).toISOString();

  /* The files pane reads its own shape out of `document`, where labels and rulesets
     read theirs off named fields - so a fixture that answered every kind with the same
     empty document left the third tab drawing an empty form. This is the smallest
     document that makes it a picture of something; `Views/SyncFilesPage` is where its
     own states are laid out. */
  const FILES_DOCUMENT = {
    files: [
      {
        path: 'CONTRIBUTING.md',
        content: '# Contributing\n\nOpen a pull request against `{{DEFAULT_BRANCH}}`.\n',
      },
    ],
    retired: ['.github/stale.yml'],
  };

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
    document: kind === 'files' ? FILES_DOCUMENT : {},
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
    discardPlan: async () => {},
    rulesetHref: (name: string) => `#/sync/rulesets/${name}`,
    onOpenRuleset: () => {},
    fileHref: (path: string) => `#/sync/files/${path}`,
    onOpenFile: () => {},
    fetchFilesContext: async () => ({ repositories: 0, covered: 0, known_paths: [], merges: [] }),
    fetchOverride: async () => {
      throw new Error('not in this story');
    },
    saveOverride: async () => {
      throw new Error('not in this story');
    },
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

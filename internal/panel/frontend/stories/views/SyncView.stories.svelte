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
     document that makes it a picture of something; `Views/SyncFilesForm` is where its
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

  /**
   * The same plan in another state. Seven of them exist and each carries its own
   * sentence, so the fixture varies the state and nothing else - what changes
   * between these stories is the words, which is the thing being looked at.
   */
  const planIn = (state: SyncPlan['state'], over: Partial<SyncPlan> = {}): SyncPlan => ({
    ...PLAN,
    state,
    ...over,
  });

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

<!--
  Approved and handed to the service. Nothing is left to press, so the button is gone
  rather than disabled: the decision has been made and this is a report of it.
-->
<Story name="Approved">
  {#snippet template(args)}
    <Seeded>
      <SyncView
        {...args}
        fetchPlan={async () => ({ plan: planIn('approved', { approved_at: at(-60_000) }) })}
      />
    </Seeded>
  {/snippet}
</Story>

<!-- Running now, somewhere else. The rows are the same rows; only the sentence moves. -->
<Story name="Being applied">
  {#snippet template(args)}
    <Seeded>
      <SyncView {...args} fetchPlan={async () => ({ plan: planIn('applying') })} />
    </Seeded>
  {/snippet}
</Story>

<!-- Finished, and every row with it. -->
<Story name="Applied">
  {#snippet template(args)}
    <Seeded>
      <SyncView
        {...args}
        fetchPlan={async () => ({
          plan: planIn('applied', {
            actions: PLAN.actions.map((action) => ({ ...action, state: 'applied' as const })),
            approved_at: at(-4 * 60_000),
            finished_at: at(-3 * 60_000),
          }),
        })}
      />
    </Seeded>
  {/snippet}
</Story>

<!--
  "The rows below say which" is a promise, so this is the story that keeps it. Both
  failure lines are here: one action GitHub refused, and one that was never tried
  because an earlier kind failed first.
-->
<Story name="Some of it failed">
  {#snippet template(args)}
    <Seeded>
      <SyncView
        {...args}
        fetchPlan={async () => ({
          plan: planIn('failed', {
            counts: { create: 1, update: 1, delete: 0 },
            actions: [
              {
                repository: 'platform-infra',
                kind: 'rulesets',
                operation: 'update',
                subject: 'main',
                state: 'failed',
                error: 'GitHub refused the write: 422 invalid bypass actor',
              },
              {
                repository: 'platform-infra',
                kind: 'labels',
                operation: 'create',
                subject: 'chore',
                state: 'skipped',
                blocker: 'rulesets',
              },
            ],
            approved_at: at(-9 * 60_000),
            finished_at: at(-8 * 60_000),
          }),
        })}
      />
    </Seeded>
  {/snippet}
</Story>

<!--
  Somebody saved while this was on screen, so it describes a world that has moved. The
  pair worth telling apart: this one is somebody else's doing.
-->
<Story name="Overtaken">
  {#snippet template(args)}
    <Seeded>
      <SyncView {...args} fetchPlan={async () => ({ plan: planIn('stale') })} />
    </Seeded>
  {/snippet}
</Story>

<!-- And this one is nobody's: it sat long enough that the offer lapsed. -->
<Story name="Expired">
  {#snippet template(args)}
    <Seeded>
      <SyncView
        {...args}
        fetchPlan={async () => ({ plan: planIn('expired', { expires_at: at(-5 * 60_000) }) })}
      />
    </Seeded>
  {/snippet}
</Story>

<!--
  A plan that deletes. Removal is off unless an operator switched it on, and it is the
  one row that destroys something a person may have made by hand - so it is drawn to be
  found without reading the list.
-->
<Story name="A plan that removes">
  {#snippet template(args)}
    <Seeded>
      <SyncView
        {...args}
        fetchPlan={async () => ({
          plan: planIn('computed', {
            counts: { create: 0, update: 0, delete: 2 },
            actions: [
              {
                repository: 'smyklot',
                kind: 'labels',
                operation: 'delete',
                subject: 'wontfix',
                before: 'ffffff',
                state: 'pending',
              },
              {
                repository: 'platform-infra',
                kind: 'files',
                operation: 'delete',
                subject: '.github/stale.yml',
                state: 'pending',
              },
            ],
          }),
        })}
      />
    </Seeded>
  {/snippet}
</Story>

<!--
  Nothing could be read, so there is no form to hang the failure on. It used to hang on
  the labels plate - the one part drawn whether or not anything had loaded - and a
  failure with nowhere to go is a page that comes up blank and says why nowhere.
-->
<Story name="Nothing loaded">
  {#snippet template(args)}
    <Seeded>
      <SyncView
        {...args}
        fetchConfig={async () => {
          throw new Error('the panel could not reach the service');
        }}
        fetchPlan={async () => {
          throw new Error('the panel could not reach the service');
        }}
      />
    </Seeded>
  {/snippet}
</Story>

<!--
  Mid-approval. The only state on this page a reader cannot reach by looking, because
  it exists between a press and an answer - so the story presses, and the answer never
  comes. Measured beside `With a plan`: the button goes from 151.51px to 91.66px on the
  press, so it collapses under the pointer at the moment somebody is watching it.
-->
<Story
  name="Approving"
  play={async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'Apply these changes' }));
  }}
>
  {#snippet template(args)}
    <Seeded>
      <SyncView {...args} approvePlan={() => new Promise<{ plan: SyncPlan }>(() => {})} />
    </Seeded>
  {/snippet}
</Story>

<!--
  The approval was refused. Worth looking at rather than assuming: the failure is
  written to the same field the labels form reads, so it surfaces under Labels rather
  than beside the plan it belongs to.
-->
<Story
  name="Approval refused"
  play={async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'Apply these changes' }));
  }}
>
  {#snippet template(args)}
    <Seeded>
      <SyncView
        {...args}
        approvePlan={async () => {
          throw new Error('this plan no longer describes the configuration; it was recomputed');
        }}
      />
    </Seeded>
  {/snippet}
</Story>

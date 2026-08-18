<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import RepositorySyncPane from '#lib/components/RepositorySyncPane.svelte';
  import type { SyncOverride } from '#lib/types.js';

  /*
   * Fixed rather than `Date.now()`, and every timestamp below is an offset from it.
   * The pane prints how long ago the planner last refused, so a clock that moved
   * between two renders would make the same story read differently each time.
   */
  const NOW = Date.parse('2026-08-18T09:00:00Z');
  const ago = (ms: number): string => new Date(NOW - ms).toISOString();

  const STORED: SyncOverride = {
    kind: 'files',
    enabled: null,
    document: {
      merges: [
        {
          path: 'renovate.json',
          strategy: 'deep-merge',
          overrides: {
            timezone: 'Europe/Warsaw',
            schedule: ['* 4 * * 6'],
            ignorePaths: ['crates/harness-codex-acp/**'],
          },
          arrays: [{ path: '$.ignorePaths', strategy: 'append' }],
          deduplicate: true,
        },
        {
          path: 'CONTRIBUTING.md',
          strategy: 'markdown',
          sections: [
            {
              action: 'after',
              heading: '### Prerequisites',
              content: '### Project setup\n\nRun `mise install`.',
            },
            {
              action: 'patch',
              heading: '### Making Changes',
              patches: [{ find: 'make check', replace: 'mise run check' }],
            },
          ],
        },
      ],
    },
    revision: 1,
    updated_by: 'bart',
    updated_at: ago(2 * 60 * 60_000),
    unreadable: false,
  };

  const { Story } = defineMeta({
    title: 'Views/RepositorySyncPane',
    component: RepositorySyncPane,
    argTypes: {
      readOnly: { control: 'boolean' },
      saving: { control: 'boolean' },
    },
    args: {
      stored: STORED,
      readOnly: false,
      saving: false,
      now: NOW,
      saveProblem: null,
      onSave: fn(),
    },
  });
</script>

<!--
  One repository's answer about the files the organization keeps in step: whether the
  sync runs here at all, and what this repository adjusts about it. Enablement is
  inherited here, which is the default and the state most repositories are in.
-->
<Story name="One adjustment" />

<!-- Nothing said yet. The pane has to offer a way in rather than show a void. -->
<Story
  name="Nothing adjusted"
  args={{
    stored: { kind: 'files', enabled: null, document: {}, revision: 0, unreadable: false },
  }}
/>

<!-- Turned off for this repository alone, against an installation that syncs. -->
<Story name="Switched off here" args={{ stored: { ...STORED, enabled: false } }} />

<!-- And turned on here, which is the other way an answer stops being inherited. -->
<Story name="Switched on here" args={{ stored: { ...STORED, enabled: true } }} />

<!--
  The planner is not syncing this repository, and this notice is the only account of
  it anybody sees. Distinct from a save that failed: this one is about the world, and
  it carries when it was found.
-->
<Story
  name="Refused by the planner"
  args={{
    stored: {
      ...STORED,
      problem:
        'these files cannot be composed: docs/guide.md cannot be written ' +
        'because docs is not a directory in this repository',
      problem_at: ago(4 * 60_000),
    },
  }}
/>

<!-- Somebody else saved first. The pane says so and keeps what was typed. -->
<Story
  name="Save refused"
  args={{ saveProblem: 'this repository changed; reload and try again' }}
/>

<Story name="Saving" args={{ saving: true }} />

<!--
  A document written by a newer build of the service. Nothing here was shown, so
  nothing here may be saved over it.
-->
<Story name="Unreadable" args={{ stored: { ...STORED, unreadable: true } }} />

<Story name="Read only" args={{ readOnly: true }} />

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
    },
    args: {
      stored: STORED,
      repositoryId: 'repository-1',
      readOnly: false,
      now: NOW,
      onChange: fn(),
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

<Story name="Unsaved document" args={{ dirtyDocument: true }} />

<Story name="Unsaved enablement" args={{ dirtyEnabled: true }} />

<!--
  A document written by a newer build of the service. Nothing here was shown, so
  nothing here may be saved over it.
-->
<Story name="Unreadable" args={{ stored: { ...STORED, unreadable: true } }} />

<Story name="Read only" args={{ readOnly: true }} />

<!--
  A row naming a file and nothing else. `Spec.Empty()` does not rescue it - the short
  circuit for an empty merge lives in `Apply`, not on the save path - so the engine
  refuses it, and the pane says so under the box rather than after a round trip.
-->
<Story
  name="Sets nothing"
  args={{
    stored: {
      ...STORED,
      document: { merges: [{ path: 'renovate.json', strategy: 'deep-merge' }] },
    },
  }}
/>

<!--
  A list rule whose path is not a path. Nine refusals are written here and this is the
  shape they share: which rule, of which file, and what is wrong with it in the words
  somebody reading the row would use.
-->
<Story
  name="A rule path that is not one"
  args={{
    stored: {
      ...STORED,
      document: {
        merges: [
          {
            path: 'renovate.json',
            strategy: 'deep-merge',
            overrides: { ignorePaths: ['docs/**'] },
            arrays: [{ path: '$ignorePaths', strategy: 'append' }],
          },
        ],
      },
    },
  }}
/>

<!--
  The refusal only this pane can make: a rule for a list the overrides never set. The
  engine would refuse it for every template, always, and the pane holds both documents
  so it can say which key is missing instead of naming the file and stopping.
-->
<Story
  name="A rule with no list"
  args={{
    stored: {
      ...STORED,
      document: {
        merges: [
          {
            path: 'renovate.json',
            strategy: 'deep-merge',
            overrides: { timezone: 'Europe/Warsaw' },
            arrays: [{ path: '$.ignorePaths', strategy: 'append' }],
          },
        ],
      },
    },
  }}
/>

<!--
  Overrides that are not an object. Reached by typing rather than by a fixture, because
  the stored document cannot hold it - the server decodes strictly, so this is a state
  only the box in front of somebody can be in.
-->
<Story
  name="Overrides that are not an object"
  args={{
    stored: {
      ...STORED,
      document: {
        merges: [{ path: 'renovate.json', strategy: 'deep-merge', overrides: { timezone: 'UTC' } }],
      },
    },
  }}
  play={async ({ canvas, userEvent }) => {
    const box = await canvas.findByLabelText('What this repository sets');

    // Typed onto the end rather than over a cleared box. `clear()` leaves the element
    // without the browser's dirty-value flag, and the box reports on `change` - so a
    // cleared-then-retyped box blurs silently and the story shows nothing.
    // `[[` is one literal bracket: userEvent reads a bare `[` as a key descriptor.
    await userEvent.type(box, '[[1]');
    // The refusal arrives when the caret leaves, not while somebody is still typing.
    await userEvent.tab();
  }}
/>

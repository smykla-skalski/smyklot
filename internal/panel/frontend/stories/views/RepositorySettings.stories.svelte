<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import RepositorySettings from '#lib/components/RepositorySettings.svelte';
  import type { SyncOverride } from '#lib/types.js';
  import { REPOSITORY, REPOSITORY_DETAIL } from '../support/fixtures.js';

  /* Fixed, and every timestamp an offset from it: the sync pane prints how long ago
     the override was last written, so a moving clock would make one story read
     differently on every render. The same instant `RepositorySyncPane`'s own stories
     use. */
  const NOW = Date.parse('2026-08-18T09:00:00Z');

  const SYNC_OVERRIDE: SyncOverride = {
    kind: 'files',
    enabled: null,
    document: {
      merges: [
        {
          path: 'renovate.json',
          strategy: 'deep-merge',
          overrides: { timezone: 'Europe/Warsaw' },
        },
      ],
    },
    revision: 1,
    updated_by: 'bart',
    updated_at: new Date(NOW - 2 * 60 * 60_000).toISOString(),
    unreadable: false,
  };

  const base = {
    repository: REPOSITORY,
    detail: REPOSITORY_DETAIL,
    section: 'behavior' as const,
    backHref: '#/repositories',
    onBack: fn(),
    onSection: fn(),
    onBypass: fn(),
    onSaveConfig: async () => {},
    onResetMigration: fn(),
    now: NOW,
    onSaveSync: fn(),
  };

  const { Story } = defineMeta({
    title: 'Views/RepositorySettings',
    component: RepositorySettings,
    argTypes: {
      section: { control: 'inline-radio', options: ['file', 'behavior', 'commands', 'sync'] },
      readOnly: { control: 'boolean' },
      busy: { control: 'boolean' },
    },
    args: base,
  });
</script>

<!--
  One repository opened from its row, on one of its three panes. The section is in the
  address, so a reload lands where the reader was.
-->
<Story name="Behaviour" />

<Story name="Commands" args={{ section: 'commands' }} />

<!-- The configuration file pane: what the repository wrote, and whether it parsed. -->
<Story name="File" args={{ section: 'file' }} />

<!-- A file the service could not read. Nothing in it is applied, and it says so. -->
<Story
  name="File is invalid"
  args={{
    section: 'file',
    repository: { ...REPOSITORY, config_file_status: 'invalid' },
    detail: {
      ...REPOSITORY_DETAIL,
      config_file_error: 'line 4: unknown key "aproved_commands"',
      repository: { ...REPOSITORY, config_file_status: 'invalid' },
    },
  }}
/>

<!-- No file at all, so every value comes from the account above it. -->
<Story
  name="No file"
  args={{
    section: 'file',
    repository: { ...REPOSITORY, config_file_status: 'missing', config_override_count: 0 },
    detail: {
      ...REPOSITORY_DETAIL,
      config_file_patch: {},
      config_patch: {},
      repository: { ...REPOSITORY, config_file_status: 'missing', config_override_count: 0 },
    },
  }}
/>

<Story name="Read only" args={{ readOnly: true }} />

<Story name="Saving" args={{ busy: true }} />

<!-- The detail has not arrived yet; the panes stand in for it. -->
<Story name="Loading" args={{ detail: undefined }} />

<Story name="With a problem" args={{ failure: 'GitHub refused the write: 403' }} />

<!--
  The fourth pane, which arrived with the org-wide file sync. It is the only one whose
  contents are read when it opens rather than with the rest of the page, so `undefined`
  is a real state here and not a slow render - see "Still reading" below.
-->
<Story name="Sync" args={{ section: 'sync', syncOverride: SYNC_OVERRIDE }} />

<!-- Nothing said here, so the organization's own answer stands. -->
<Story
  name="Sync inherits"
  args={{ section: 'sync', syncOverride: { ...SYNC_OVERRIDE, enabled: null, document: {} } }}
/>

<!-- Turned off for this repository alone, whatever the organization keeps in step. -->
<Story
  name="Sync switched off"
  args={{ section: 'sync', syncOverride: { ...SYNC_OVERRIDE, enabled: false } }}
/>

<!-- The read has not come back. The pane has nothing to draw, which is not the same
     as a repository that says nothing. -->
<Story name="Sync still reading" args={{ section: 'sync', syncOverride: undefined }} />

<!-- A read that did not answer leaves the pane empty; a refused save leaves it
     drawing what the reader typed. They are two problems and the component takes
     them separately. -->
<Story
  name="Sync unreadable"
  args={{
    section: 'sync',
    syncOverride: undefined,
    syncReadProblem: 'The override could not be read: 502',
  }}
/>

<Story
  name="Sync save refused"
  args={{
    section: 'sync',
    syncOverride: SYNC_OVERRIDE,
    syncSaveProblem: 'GitHub refused the write: 403',
  }}
/>

<Story
  name="Sync saving"
  args={{ section: 'sync', syncOverride: SYNC_OVERRIDE, syncSaving: true }}
/>

<!--
  Root manages somebody else's installation, and sync has no Root address - so the
  pane is not offered there at all. `sections` is what says so, and this is the story
  that shows the switch with three panes rather than four.
-->
<Story
  name="Sync not offered"
  args={{ sections: ['file', 'behavior', 'commands'] as const, section: 'behavior' }}
/>

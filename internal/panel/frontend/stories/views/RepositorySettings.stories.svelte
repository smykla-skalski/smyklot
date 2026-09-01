<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import RepositorySettings from '#lib/components/RepositorySettings.svelte';
  import type { SyncOverride } from '#lib/types.js';
  import { REPOSITORY, REPOSITORY_DETAIL } from '../support/fixtures.js';

  /* Fixed, and every timestamp an offset from it: the sync card prints how long ago
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
    backHref: '#/repositories',
    onBack: fn(),
    onChange: fn(),
    onResetMigration: fn(),
    now: NOW,
    syncOverride: SYNC_OVERRIDE,
    onChangeSync: fn(),
  };

  const { Story } = defineMeta({
    title: 'Views/RepositorySettings',
    component: RepositorySettings,
    argTypes: {
      offersSync: { control: 'boolean' },
      readOnly: { control: 'boolean' },
      busy: { control: 'boolean' },
    },
    args: base,
  });
</script>

<!--
  One repository opened from its row: the whole page in one scroll. It used to be five
  panes behind a switch, which made a reader press four times to see what one repository
  is set to and hid from them that most of those panes were empty.
-->
<Story name="Default" />

<!-- A file the service could not read. Nothing in it is applied, and it says so. -->
<Story
  name="File is invalid"
  args={{
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

<Story name="Resetting migration" args={{ busy: true }} />

<!-- The detail has not arrived yet; the page stands in for it. -->
<Story name="Loading" args={{ detail: undefined }} />

<Story name="With a problem" args={{ failure: 'GitHub refused the write: 403' }} />

<!-- Nothing said here, so the organization's own answer stands. -->
<Story
  name="Sync inherits"
  args={{ syncOverride: { ...SYNC_OVERRIDE, enabled: null, document: {} } }}
/>

<!-- Turned off for this repository alone, whatever the organization keeps in step. -->
<Story name="Sync switched off" args={{ syncOverride: { ...SYNC_OVERRIDE, enabled: false } }} />

<!-- The read has not come back. The card has nothing to draw, which is not the same
     as a repository that says nothing. -->
<Story name="Sync still reading" args={{ syncOverride: undefined }} />

<!-- A read that did not answer leaves the card empty. -->
<Story
  name="Sync unreadable"
  args={{
    syncOverride: undefined,
    syncReadProblem: 'The override could not be read: 502',
  }}
/>

<Story
  name="Sync with unsaved document"
  args={{
    dirtyControls: [`repositories.${REPOSITORY.id}.sync.files.document`],
  }}
/>

<!--
  Root manages somebody else's workspace, and sync has no Root address - so the card
  is not drawn there at all. `offersSync` is what says so.
-->
<Story name="Sync not offered" args={{ offersSync: false }} />

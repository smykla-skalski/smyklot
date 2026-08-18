<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import RepositorySettings from '#lib/components/RepositorySettings.svelte';
  import { REPOSITORY, REPOSITORY_DETAIL } from '../support/fixtures.js';

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
  };

  const { Story } = defineMeta({
    title: 'Views/RepositorySettings',
    component: RepositorySettings,
    argTypes: {
      section: { control: 'inline-radio', options: ['file', 'behavior', 'commands'] },
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

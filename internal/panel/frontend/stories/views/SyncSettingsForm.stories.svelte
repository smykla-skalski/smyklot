<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SyncSettingsForm from '#lib/components/SyncSettingsForm.svelte';

  const STORED = {
    has_issues: true,
    has_wiki: false,
    has_projects: false,
    allow_squash_merge: true,
    delete_branch_on_merge: true,
    default_branch: 'main',
  };

  const { Story } = defineMeta({
    title: 'Views/SyncSettingsForm',
    component: SyncSettingsForm,
    argTypes: {
      enabled: { control: 'boolean' },
      unreadable: { control: 'boolean' },
      readOnly: { control: 'boolean' },
      saving: { control: 'boolean' },
    },
    args: {
      stored: STORED,
      enabled: true,
      unreadable: false,
      readOnly: false,
      saving: false,
      onSave: fn(),
    },
  });
</script>

<!--
  The repository settings every repository in an installation should share. A field is
  either a switch - a boolean in the document - or a choice among GitHub's own words.
-->
<Story name="Enabled" />

<Story name="Switched off" args={{ enabled: false }} />

<Story name="Saving" args={{ saving: true }} />

<!-- The file on GitHub cannot be parsed, so nothing here can be trusted to save. -->
<Story name="Unreadable" args={{ unreadable: true }} />

<Story name="Read only" args={{ readOnly: true }} />

<Story name="With a problem" args={{ problem: 'GitHub refused the write: 403' }} />

<!-- Nothing stored yet, so every field falls back to what GitHub already has. -->
<Story name="Nothing stored" args={{ stored: {} }} />

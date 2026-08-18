<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SyncFilesForm from '#lib/components/SyncFilesForm.svelte';

  const STORED = {
    files: [
      {
        path: 'CONTRIBUTING.md',
        content:
          '# Contributing\n\nOpen a pull request against `{{DEFAULT_BRANCH}}`.\n' +
          'Every change needs a review from a code owner.\n',
      },
      {
        path: 'renovate.json',
        content: '{\n  "extends": ["config:recommended"],\n  "timezone": "UTC"\n}\n',
      },
    ],
    retired: ['.github/workflows/sync-trigger.yml'],
    excludes: ['LICENSE'],
  };

  const { Story } = defineMeta({
    title: 'Views/SyncFilesForm',
    component: SyncFilesForm,
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
      unavailable: '',
      problem: null,
      readOnly: false,
      saving: false,
      onSave: fn(),
    },
  });
</script>

<!--
  The files an installation expects every repository to carry. A card per file, the
  path above what it should say, so the height of a card is the height of the
  template - which is the whole reason this is seeded rather than shown empty.
-->
<Story name="Two files" />

<!-- Nothing configured yet: the form has to offer a way in rather than show a void. -->
<Story name="Empty" args={{ stored: {} }} />

<!--
  Retired paths with no template beside them. Naming a path is the only way to have
  it removed and naming it is the consent, so this is the shape that deletes.
-->
<Story name="Only retirements" args={{ stored: { retired: STORED.retired } }} />

<Story name="Switched off" args={{ enabled: false }} />

<Story name="Saving" args={{ saving: true }} />

<Story name="Unreadable" args={{ unreadable: true }} />

<Story name="Read only" args={{ readOnly: true }} />

<!--
  The planner refused these files, and the reason is the only account of it anybody
  sees. Long on purpose: it wraps, and it has to stay readable when it does.
-->
<Story
  name="Refused"
  args={{
    problem:
      'these files cannot be composed: docs/guide.md cannot be written ' +
      'because docs is not a directory in this repository',
  }}
/>

<!-- The installation is not synchronizing files at all, so there is nothing to set. -->
<Story name="Unavailable" args={{ unavailable: 'this installation does not sync files' }} />

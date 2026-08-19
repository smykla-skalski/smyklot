<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SyncFilesForm from '#lib/components/SyncFilesForm.svelte';
  import type { SyncState } from '#lib/components/StateMark.svelte';
  import type { SyncOverrideRow } from '#lib/types.js';

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

  /* One repository adjusts a template, which is what makes a row say "merges"
     rather than "replaces" - the strategy is decided by the repository, not by
     the installation. */
  const ADJUSTMENTS = [
    {
      repository_id: '4001',
      repository_name: 'af',
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
      unreadable: false,
      updated_at: '2026-08-18T09:00:00Z',
    },
  ] as unknown as SyncOverrideRow[];

  const PATHS = [
    { path: 'renovate.json', repositories: 24 },
    { path: 'CONTRIBUTING.md', repositories: 12 },
    { path: '.github/workflows/test.yaml', repositories: 25 },
    { path: '.github/CODEOWNERS', repositories: 25 },
    { path: 'LICENSE', repositories: 20 },
  ];

  const MARKS: Record<string, { state: SyncState; label?: string }> = {
    'renovate.json': { state: 'change', label: '2 differ' },
    'CONTRIBUTING.md': { state: 'settled' },
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
      fileHref: (path: string) => `#file-${path}`,
      adjustments: ADJUSTMENTS,
      paths: PATHS,
      repositories: 25,
      markOf: (path: string) => MARKS[path],
      onSave: fn(),
    },
  });
</script>

<!--
  The files an installation expects every repository to carry, as a list of named
  things: two levels and no deeper, and a row opens the file's own page. How a
  file arrives is read from the repositories rather than from the template - the
  installation says what it should say, and a repository says how its own differs.
-->
<Story name="Two files" />

<!-- No plan has been worked out, so no row claims to be in step. -->
<Story name="Before a plan" args={{ markOf: () => undefined }} />

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

<!--
  The installation has not granted what file sync needs. The words are the server's own:
  `Unavailable.Reason` names the permission so the notice can say which one to grant.
-->
<Story
  name="Unavailable"
  args={{ unavailable: 'Smyklot has not been granted contents access, which files sync needs' }}
/>

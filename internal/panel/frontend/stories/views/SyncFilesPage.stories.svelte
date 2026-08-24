<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SyncFilesPage from '#lib/components/SyncFilesPage.svelte';
  import type { SyncConfig, SyncFilesContext, SyncPlan, SyncStatus } from '#lib/types.js';

  const NOW = Date.UTC(2026, 7, 18, 12, 0, 0);
  const days = (count: number): string => new Date(NOW - count * 86_400_000).toISOString();

  const CONFIG: SyncConfig = {
    kind: 'files',
    enabled: true,
    labels: [],
    allow_removal: false,
    excludes: [],
    revision: 4,
    updated_by: 'bart',
    updated_at: days(2),
    digest: 'digest',
    document: {
      files: [
        { path: 'renovate.json', content: '{}', updated_at: days(2), updated_by: 'bartsmykla' },
        { path: '.github/workflows/ci.yaml', content: 'name: test', updated_at: days(5) },
        { path: 'CONTRIBUTING.md', content: '# Contributing', updated_at: days(21) },
        { path: '.github/CODEOWNERS', content: '* @maintainers', updated_at: days(21) },
        { path: 'LICENSE', content: 'Apache License 2.0', updated_at: days(120) },
      ],
      retired: ['.github/stale.yml'],
      excludes: ['LICENSE-*'],
    },
    unreadable: false,
    unavailable: '',
  };

  const CONTEXT: SyncFilesContext = {
    repositories: 25,
    covered: 23,
    known_paths: [
      { path: '.github/workflows/ci.yaml', repositories: 25 },
      { path: '.github/workflows/release.yaml', repositories: 18 },
      { path: 'renovate.json', repositories: 21 },
      { path: 'CONTRIBUTING.md', repositories: 24 },
      { path: 'README.md', repositories: 25 },
      { path: 'LICENSE', repositories: 25 },
      { path: 'Makefile', repositories: 14 },
      { path: '.editorconfig', repositories: 17 },
    ],
    merges: [
      {
        repository: 'af',
        repository_id: '9101',
        path: 'renovate.json',
        merge: { path: 'renovate.json', overrides: { timezone: 'Europe/Warsaw' } },
      },
      {
        repository: 'afi',
        repository_id: '9102',
        path: 'renovate.json',
        merge: { path: 'renovate.json', overrides: {} },
      },
      {
        repository: 'harness',
        repository_id: '9103',
        path: 'renovate.json',
        merge: { path: 'renovate.json', overrides: {} },
      },
      {
        repository: 'smyklot',
        repository_id: '4001',
        path: 'CONTRIBUTING.md',
        merge: { path: 'CONTRIBUTING.md', strategy: 'markdown', sections: [{}] },
      },
    ],
  };

  const PLAN: SyncPlan = {
    id: 'plan-1',
    trigger: 'sweep',
    state: 'computed',
    execution_stage: 'Waiting for approval',
    digest: 'digest',
    counts: { create: 1, update: 0, delete: 1 },
    actions: [
      {
        repository: 'af',
        kind: 'files',
        operation: 'create',
        subject: 'renovate.json',
        state: 'pending',
      },
      {
        repository: 'afi',
        kind: 'files',
        operation: 'update',
        subject: 'renovate.json',
        state: 'pending',
      },
      {
        repository: 'af',
        kind: 'files',
        operation: 'update',
        subject: 'CONTRIBUTING.md',
        state: 'pending',
      },
    ],
    computed_at: new Date(NOW - 12 * 60_000).toISOString(),
    expires_at: new Date(NOW + 6 * 3_600_000).toISOString(),
  };

  const STATUS: SyncStatus = {
    checked_at: new Date(NOW - 5 * 60_000).toISOString(),
    repositories: [
      {
        repository: 'smyklot-legacy',
        cells: {
          labels: { state: 'in_step' },
          settings: { state: 'in_step' },
          rulesets: { state: 'in_step' },
          files: { state: 'refused' },
        },
        reason:
          '.github/workflows/ci.yaml needs the workflows permission - grant it on the ' +
          "installation's page",
      },
    ],
  };

  const { Story } = defineMeta({
    title: 'Views/SyncFilesPage',
    component: SyncFilesPage,
    args: {
      config: CONFIG,
      context: CONTEXT,
      plan: PLAN,
      status: STATUS,
      nowMs: NOW,
      readOnly: false,
      problem: null,
      sectionHref: (section: string) => `#/sync/${section}`,
      onOpenSection: fn(),
      fileHref: (path: string) => `#/sync/files/${path}`,
      onOpenFile: fn(),
      onToggleEnabled: fn(),
      onChangeDocument: fn(),
    },
  });
</script>

<!--
  The shared files list: each template a named object one press from its own
  page, the strategy worn as a pill, adjusters and freshness in the summary,
  and the plan's verdict riding the row's end. Add a file opens the finder -
  fuzzy suggestions from what the organization's repositories already hold.
-->
<Story name="Five templates" />

<Story name="Unsaved files" args={{ dirtyDocument: true, savedDocument: {} }} />

<!-- Nothing shared yet. -->
<Story
  name="Empty"
  args={{
    config: { ...CONFIG, document: { files: [], retired: [], excludes: [] } },
    context: { ...CONTEXT, merges: [] },
    plan: null,
    status: null,
  }}
/>

<!-- A reader without write. -->
<Story name="Read only" args={{ readOnly: true }} />

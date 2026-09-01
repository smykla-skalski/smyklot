<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SyncFilePage from '#lib/components/SyncFilePage.svelte';
  import { defaultFormattingPolicy } from '#lib/formatting.js';
  import { buildSyncOverrideEditorEnvelope } from '#lib/repository-sync-override-settings.js';
  import type { SyncConfig, SyncFilesContext, SyncOverride } from '#lib/types.js';

  const NOW = Date.UTC(2026, 7, 18, 12, 0, 0);
  const POLICY = defaultFormattingPolicy();
  const days = (count: number): string => new Date(NOW - count * 86_400_000).toISOString();

  const TEMPLATE = [
    '{',
    '  "$schema": "https://docs.renovatebot.com/renovate-schema.json",',
    '  "extends": ["config:recommended"],',
    '  // Weekend runs keep review noise out of the working week',
    '  "schedule": ["* 4 * * 6"],',
    '  "timezone": "UTC",',
    '  "packageRules": [',
    '    { "matchManagers": ["gomod"], "groupName": "go modules" }',
    '  ],',
    '  "automerge": false',
    '}',
  ].join('\n');

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
        {
          path: 'renovate.json',
          content: TEMPLATE,
        },
      ],
      retired: [],
      excludes: [],
    },
    unreadable: false,
    unavailable: '',
  };

  /* The three repositories that adjust the template, named and identified the
     way the workspace's own repository list names them. */
  const ADJUSTERS = [
    { name: 'platform-infra', id: '4002' },
    { name: 'api-gateway', id: '4005' },
    { name: 'auth-service', id: '4006' },
  ];

  const AF_MERGE = {
    path: 'renovate.json',
    strategy: 'deep-merge',
    overrides: {
      schedule: ['* 4 * * 1-5'],
      timezone: 'Europe/Warsaw',
      packageRules: [{ matchManagers: ['npm'], groupName: 'frontend packages' }],
    },
    arrays: [{ path: '$.packageRules', strategy: 'append' }],
  };

  const CONTEXT: SyncFilesContext = {
    repositories: 25,
    covered: 23,
    known_paths: [],
    base_formatting: POLICY,
    repository_policies: Array.from({ length: 25 }, (_, index) => ({
      repository: ADJUSTERS[index]?.name ?? `repository-${index + 1}`,
      repository_id: ADJUSTERS[index]?.id ?? `40${String(index + 5).padStart(2, '0')}`,
      default_branch: 'main',
      base_policy: POLICY,
    })),
    merges: [
      {
        repository: 'platform-infra',
        repository_id: '4002',
        path: 'renovate.json',
        merge: AF_MERGE,
      },
      {
        repository: 'api-gateway',
        repository_id: '4005',
        path: 'renovate.json',
        merge: {
          path: 'renovate.json',
          strategy: 'deep-merge',
          overrides: { packageRules: [{ matchManagers: ['dockerfile'], groupName: 'images' }] },
          arrays: [{ path: '$.packageRules', strategy: 'append' }],
        },
      },
      {
        repository: 'auth-service',
        repository_id: '4006',
        path: 'renovate.json',
        merge: { path: 'renovate.json', strategy: 'deep-merge', overrides: { automerge: null } },
      },
    ],
  };

  const OVERRIDE: SyncOverride = {
    kind: 'files',
    enabled: null,
    document: { merges: [AF_MERGE] },
    revision: 2,
    updated_by: 'bart',
    updated_at: days(1),
    unreadable: false,
  };

  const { Story } = defineMeta({
    title: 'Views/SyncFilePage',
    component: SyncFilePage,
    args: {
      config: CONFIG,
      context: CONTEXT,
      path: 'renovate.json',
      nowMs: NOW,
      readOnly: false,
      problem: null,
      sectionHref: (section: string) => `#/sync/${section}`,
      onOpenSection: fn(),
      onChangeDocument: fn(() => true),
      fetchOverride: async () => ({
        stored: OVERRIDE,
        envelope: buildSyncOverrideEditorEnvelope(OVERRIDE),
      }),
      renderFile: async (input) => ({
        valid: true,
        content: input.draft_content,
        changed: false,
        diagnostics: [],
      }),
      onFormattingValidity: fn(),
      onChangeOverride: fn(() => true),
    },
  });
</script>

<!--
  One template's own page: the template as evidence, and each adjusting
  repository's composed copy - overridden lines wearing the managed gutter
  bar, the keys on the patch strip with the x that removes the override,
  and the list question asked where it arises. Press Edit on a repository
  row to open its adjustment.
-->
<Story name="renovate.json" />

<Story name="Unsaved template" args={{ dirtyDocument: true, savedDocument: {} }} />

<!-- Nobody adjusts it: the template is the whole story. -->
<Story name="No adjustments" args={{ context: { ...CONTEXT, merges: [] } }} />

<!-- An address naming a template that is gone says so. -->
<Story name="Not found" args={{ path: 'renamed-away.json' }} />

<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SyncFileDetail from '#lib/components/SyncFileDetail.svelte';
  import type { SyncOverrideRow } from '#lib/types.js';

  const TEMPLATE = `{
  "$schema": "https://docs.renovatebot.com/renovate-schema.json",
  "extends": ["config:recommended"],
  "schedule": ["* 4 * * 6"],
  "timezone": "UTC",
  "packageRules": [{ "matchManagers": ["gomod"] }],
  "automerge": false
}
`;

  const STORED = {
    files: [
      { path: 'renovate.json', content: TEMPLATE },
      { path: 'CONTRIBUTING.md', content: '# Contributing\n\nOpen a pull request.\n' },
    ],
  };

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
            overrides: { schedule: ['* 4 * * 1-5'], timezone: 'Europe/Warsaw' },
          },
        ],
      },
      revision: 1,
      unreadable: false,
    },
    {
      repository_id: '4002',
      repository_name: 'harness',
      kind: 'files',
      enabled: null,
      document: {
        merges: [{ path: 'renovate.json', overrides: { automerge: null } }],
      },
      revision: 2,
      unreadable: false,
    },
  ] as unknown as SyncOverrideRow[];

  const { Story } = defineMeta({
    title: 'Views/SyncFileDetail',
    component: SyncFileDetail,
    args: {
      stored: STORED,
      path: 'renovate.json',
      listHref: '#files',
      adjustments: ADJUSTMENTS,
      repositories: 25,
      updatedBy: 'bart',
      updatedAt: '2026-08-16T09:00:00Z',
      now: Date.UTC(2026, 7, 18),
      readOnly: false,
      saving: false,
      unreadable: false,
      onSave: fn(),
      onSaveAdjustment: fn(),
    },
  });
</script>

<!--
  The RESULT is the editable surface, never the adjustment. Somebody here wants
  the file their repository will hold; asking them to write a JSON merge patch
  is asking them to work out the difference in their head and type that instead.
  Press Edit on a repository to see the composed file, its overridden lines
  wearing the managed bar, and the keys it decides as chips.
-->
<Story name="A template two repositories adjust" />

<!-- Nobody adjusts it, so every repository gets it exactly as written. -->
<Story name="Nothing adjusted" args={{ adjustments: [] }} />

<!--
  A Markdown file is composed by rules a browser cannot reproduce, so an
  adjustment is named rather than drawn as a file that would be a guess.
-->
<Story name="A file this cannot compose" args={{ path: 'CONTRIBUTING.md', adjustments: [] }} />

<!-- An address written down before the template was retired. -->
<Story name="No template by that path" args={{ path: 'renovate.json5' }} />

<Story name="Read only" args={{ readOnly: true }} />

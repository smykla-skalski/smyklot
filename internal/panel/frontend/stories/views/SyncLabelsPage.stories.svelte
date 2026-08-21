<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SyncLabelsPage from '#lib/components/SyncLabelsPage.svelte';
  import type { SyncConfig } from '#lib/types.js';

  const NOW = Date.UTC(2026, 7, 18, 12, 0, 0);
  const hours = (count: number): string => new Date(NOW - count * 3_600_000).toISOString();

  /* The approved mock's five rows, word for word. */
  const CONFIG: SyncConfig = {
    kind: 'labels',
    enabled: true,
    labels: [
      { name: 'bug', color: 'd73a4a', description: 'Something is broken' },
      { name: 'enhancement', color: 'a2eeef', description: 'New behaviour somebody asked for' },
      {
        name: 'dependencies',
        color: '0e8a16',
        description: "Dependency updates, mostly Renovate's",
      },
      {
        name: 'good first issue',
        color: '7057ff',
        description: 'Small, self-contained, documented',
      },
      { name: 'chore', color: '6b7280' },
    ],
    allow_removal: false,
    excludes: ['hand-made-*'],
    revision: 3,
    updated_by: 'bart',
    updated_at: hours(2),
    digest: 'digest',
    document: {},
    unreadable: false,
    unavailable: '',
  };

  const { Story } = defineMeta({
    title: 'Views/SyncLabelsPage',
    component: SyncLabelsPage,
    args: {
      config: CONFIG,
      readOnly: false,
      problem: null,
      sectionHref: (section: string) => `#/sync/${section}`,
      onOpenSection: fn(),
      onSave: fn(async () => true),
    },
  });
</script>

<!--
  The labels page: immediate apply, per-segment editing. Press any name,
  description or colour dot and only that piece becomes its editor, in
  place; the whisper in the card head is the save receipt. Below, whether
  unlisted labels are removed, and the patterns left alone either way.
-->
<Story name="Five labels" />

<!-- A reader without write: every control stands down, the list still reads. -->
<Story name="Read only" args={{ readOnly: true }} />

<!-- The stored document could not be read: nothing shown, nothing editable,
     and the page says why rather than showing an empty list. -->
<Story name="Unreadable" args={{ config: { ...CONFIG, labels: [], unreadable: true } }} />

<!-- A permission the installation has not granted, surfaced while the kind
     is switched on and waiting on it. -->
<Story
  name="Missing permission"
  args={{
    config: {
      ...CONFIG,
      unavailable: 'Smyklot has not been granted issues access, which label sync needs',
    },
  }}
/>

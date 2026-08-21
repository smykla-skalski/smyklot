<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SyncSettingsPage from '#lib/components/SyncSettingsPage.svelte';
  import type { SyncConfig } from '#lib/types.js';

  const NOW = Date.UTC(2026, 7, 18, 12, 0, 0);
  const hours = (count: number): string => new Date(NOW - count * 3_600_000).toISOString();

  /* The approved mock's nine managed settings: Merging 4 of 6, Commit
     wording 2 of 4, Features 2 of 4, Security 1 of 3. */
  const CONFIG: SyncConfig = {
    kind: 'settings',
    enabled: true,
    labels: [],
    allow_removal: false,
    excludes: [],
    revision: 5,
    updated_by: 'bart',
    updated_at: hours(26),
    digest: 'digest',
    document: {
      allow_squash_merge: true,
      allow_merge_commit: false,
      allow_auto_merge: true,
      delete_branch_on_merge: true,
      squash_merge_commit_title: 'PR_TITLE',
      squash_merge_commit_message: 'COMMIT_MESSAGES',
      has_issues: true,
      has_wiki: false,
      secret_scanning: true,
    },
    unreadable: false,
    unavailable: '',
  };

  const { Story } = defineMeta({
    title: 'Views/SyncSettingsPage',
    component: SyncSettingsPage,
    args: {
      config: CONFIG,
      readOnly: false,
      problem: null,
      saving: false,
      sectionHref: (section: string) => `#/sync/${section}`,
      onOpenSection: fn(),
      onSave: fn(),
    },
  });
</script>

<!--
  Summary-first: the page IS the policy. Only managed settings render as
  rows - the value said in a word beside its control - and everything
  unmanaged is one sentence per group with the names as scent, one press
  from being managed. The x removes the management, never "writes the
  default"; Everything turns the unmanaged names into rows of their own.
-->
<Story name="Nine managed" />

<!-- Nothing managed anywhere: every group is one sentence. -->
<Story name="Nothing managed" args={{ config: { ...CONFIG, document: {} } }} />

<!-- A reader without write: the policy still reads, the controls stand down. -->
<Story name="Read only" args={{ readOnly: true }} />

<!-- A permission the installation has not granted, surfaced while the kind
     is switched on and waiting on it. -->
<Story
  name="Missing permission"
  args={{
    config: {
      ...CONFIG,
      unavailable: 'Smyklot has not been granted administration access, which settings sync needs',
    },
  }}
/>

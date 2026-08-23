<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SyncSaveComposer from '#lib/components/SyncSaveComposer.svelte';
  import { SyncDraftSet } from '#lib/sync-drafts.svelte.js';
  import type { SyncConfig, SyncKind } from '#lib/types.js';

  const config = (kind: SyncKind): SyncConfig => ({
    kind,
    enabled: false,
    labels: [],
    allow_removal: false,
    excludes: [],
    revision: 1,
    updated_by: 'bart',
    updated_at: new Date(0).toISOString(),
    digest: kind,
    document: {},
    unreadable: false,
    unavailable: '',
  });

  function draft(): SyncDraftSet {
    const drafts = new SyncDraftSet('target-1');
    drafts.adopt((['labels', 'settings', 'rulesets', 'files'] as const).map(config));
    drafts.stage('labels', {
      enabled: true,
      labels: [{ name: 'ci/green', color: '00ff00' }],
      allow_removal: false,
      excludes: [],
    });
    drafts.stage('settings', { enabled: true, document: { visibility: 'private' } });
    return drafts;
  }

  const dirty = draft();
  const invalid = draft();
  invalid.problem = 'A label name is required';
  invalid.invalidKind = 'labels';
  const saved = new SyncDraftSet('target-1');
  saved.notice = 'Saved. Reconciliation creates a plan only when repositories need changes.';

  const base = {
    drafts: dirty,
    readOnly: false,
    onSave: fn(),
    onReload: fn(),
    sectionHref: (kind: SyncKind) => `#/sync/${kind}`,
    onOpenSection: fn(),
  };

  const { Story } = defineMeta({
    title: 'Views/SyncSaveComposer',
    component: SyncSaveComposer,
    args: base,
  });
</script>

<Story name="Two changed sections" />
<Story name="Validation failure" args={{ drafts: invalid }} />
<Story name="Saved" args={{ drafts: saved }} />

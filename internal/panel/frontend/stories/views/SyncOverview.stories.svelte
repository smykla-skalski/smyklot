<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SyncOverview from '#lib/components/SyncOverview.svelte';
  import type {
    SyncCell,
    SyncConfig,
    SyncKind,
    SyncPlan,
    SyncRepositoryStatus,
    SyncStatus,
  } from '#lib/types.js';

  /* The approved mock's fleet, verbatim: 25 repositories, the three drifted
     rows mirroring the plan page exactly - af 2/2/0/2, afi 0/4/1/0,
     harness 2/1/0/0 - one refusal with its reason on the row, and two
     repositories with kinds switched off. */
  type Row = [string, ...(number | 'off' | 'ref')[]];
  const FLEET: Row[] = [
    ['.github', 0, 0, 0, 0],
    ['af', 2, 2, 0, 2],
    ['afi', 0, 4, 1, 0],
    ['harness', 2, 1, 0, 0],
    ['smyklot-legacy', 0, 0, 0, 'ref'],
    ['dotfiles', 'off', 0, 0, 'off'],
    ['sai', 0, 0, 0, 0],
    ['klaudiush', 0, 0, 0, 0],
    ['smyklot', 0, 0, 0, 0],
    ['orca', 0, 0, 0, 0],
    ['archive-old', 'off', 'off', 'off', 'off'],
    ['docs', 0, 0, 0, 0],
    ['infra', 0, 0, 0, 0],
    ['charts', 0, 0, 0, 0],
    ['actions', 0, 0, 0, 0],
    ['tooling', 0, 0, 0, 0],
    ['bench', 0, 0, 0, 0],
    ['probe', 0, 0, 0, 0],
    ['relay', 0, 0, 0, 0],
    ['skald', 0, 0, 0, 0],
    ['forge', 0, 0, 0, 0],
    ['mirror', 0, 0, 0, 0],
    ['quill', 0, 0, 0, 0],
    ['spore', 0, 0, 0, 0],
    ['weft', 0, 0, 0, 0],
  ];
  const KINDS: SyncKind[] = ['labels', 'settings', 'rulesets', 'files'];

  function cell(value: number | 'off' | 'ref'): SyncCell {
    if (value === 'off') return { state: 'off' };
    if (value === 'ref') return { state: 'refused' };
    return value > 0 ? { state: 'pending', changes: value } : { state: 'in_step' };
  }

  const REFUSAL =
    '.github/workflows/ci.yaml needs the workflows permission - grant it on the ' +
    "installation's page";

  const repositories: SyncRepositoryStatus[] = FLEET.map(([name, ...cells]) => ({
    repository: name,
    cells: Object.fromEntries(KINDS.map((kind, at) => [kind, cell(cells[at] ?? 0)])) as Record<
      SyncKind,
      SyncCell
    >,
    ...(name === 'af' ? { removals: 1 } : {}),
    ...(name === 'smyklot-legacy' ? { reason: REFUSAL } : {}),
  }));

  const NOW = Date.UTC(2026, 7, 18, 12, 0, 0);
  const minutes = (count: number): string => new Date(NOW - count * 60_000).toISOString();
  const hours = (count: number): string => new Date(NOW - count * 3_600_000).toISOString();

  const STATUS: SyncStatus = { checked_at: minutes(5), repositories };

  function config(kind: SyncKind, overrides: Partial<SyncConfig>): SyncConfig {
    return {
      kind,
      enabled: true,
      labels: [],
      allow_removal: false,
      excludes: [],
      revision: 3,
      updated_by: 'bart',
      updated_at: hours(2),
      digest: 'digest',
      document: {},
      unreadable: false,
      unavailable: '',
      ...overrides,
    };
  }

  const CONFIGS: Partial<Record<SyncKind, SyncConfig>> = {
    labels: config('labels', {
      labels: [
        { name: 'bug', color: 'd73a4a' },
        { name: 'dependencies', color: '0e8a16' },
        { name: 'good first issue', color: '7057ff' },
        { name: 'security', color: 'b60205' },
        { name: 'docs', color: '0075ca' },
      ],
      updated_at: hours(2),
    }),
    settings: config('settings', {
      /* Flat, GitHub's own keys at the top level - the shape the settings
         form stores. Nine managed of the catalogue's seventeen. */
      document: {
        allow_merge_commit: false,
        allow_squash_merge: true,
        allow_rebase_merge: false,
        allow_auto_merge: true,
        delete_branch_on_merge: true,
        allow_update_branch: true,
        has_wiki: false,
        has_discussions: false,
        squash_merge_commit_title: 'PR_TITLE',
      },
      updated_at: hours(26),
    }),
    rulesets: config('rulesets', {
      document: {
        rulesets: [
          { name: 'main-protection', enforcement: 'active' },
          { name: 'release-tags', enforcement: 'evaluate' },
        ],
      },
      updated_at: hours(72),
    }),
    files: config('files', {
      document: {
        files: [
          { path: '.github/renovate.json' },
          { path: '.github/CODEOWNERS' },
          { path: '.github/workflows/ci.yaml' },
          { path: 'CONTRIBUTING.md' },
          { path: '.editorconfig' },
        ],
        retired: ['.github/stale.yml'],
      },
      updated_at: minutes(20),
    }),
  };

  const PLAN: SyncPlan = {
    id: 'plan-1',
    trigger: 'sweep',
    state: 'computed',
    execution_stage: 'Waiting for approval',
    digest: 'digest',
    counts: { create: 8, update: 5, delete: 1 },
    actions: [],
    computed_at: minutes(12),
    expires_at: new Date(NOW + 6 * 3_600_000).toISOString(),
  };

  const { Story } = defineMeta({
    title: 'Views/SyncOverview',
    component: SyncOverview,
    args: {
      status: STATUS,
      plan: PLAN,
      configs: CONFIGS,
      nowMs: NOW,
      sectionHref: (section: string) => `#/sync/${section}`,
      onOpenSection: fn(),
      onToggleKind: fn(),
      readOnly: false,
    },
  });
</script>

<!--
  The sync overview: the verdict, the fleet as a board of keycap tiles with a
  legend that is key AND filter, the out-of-step list with each reason on its
  row, and one card per kind whose strip repeats the board's slots in the
  board's order.
-->
<Story name="Out of step" />

<!-- Everything settled: the hero says so, and the plan chrome stands down. -->
<Story
  name="All in step"
  args={{
    status: {
      checked_at: STATUS.checked_at,
      repositories: repositories.map((row) => ({
        repository: row.repository,
        cells: Object.fromEntries(KINDS.map((kind) => [kind, { state: 'in_step' }])) as Record<
          SyncKind,
          SyncCell
        >,
      })),
    },
    plan: null,
  }}
/>

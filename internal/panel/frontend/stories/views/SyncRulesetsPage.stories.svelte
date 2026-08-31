<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SyncRulesetsPage from '#lib/components/SyncRulesetsPage.svelte';
  import type { SyncConfig, SyncPlan } from '#lib/types.js';

  import { SYNC_STATUS } from '../support/fixtures.js';

  const NOW = Date.UTC(2026, 7, 18, 12, 0, 0);
  const at = (offset: number): string => new Date(NOW + offset).toISOString();

  /* The approved mock's two: main-protection active with six rules and two
     bypass actors, release-tags still evaluating. */
  const CONFIG: SyncConfig = {
    kind: 'rulesets',
    enabled: true,
    labels: [],
    allow_removal: false,
    excludes: [],
    revision: 2,
    updated_by: 'bart',
    updated_at: at(-3 * 24 * 3_600_000),
    digest: 'digest',
    document: {
      rulesets: [
        {
          name: 'main-protection',
          target: 'branch',
          enforcement: 'active',
          conditions: { include: ['~DEFAULT_BRANCH'], exclude: [] },
          bypass_actors: [
            { actor_id: 5, actor_type: 'RepositoryRole', bypass_mode: 'always' },
            { actor_id: 1216238, actor_type: 'Integration', bypass_mode: 'pull_request' },
          ],
          rules: {
            deletion: true,
            non_fast_forward: true,
            required_linear_history: true,
            required_signatures: true,
            pull_request: {
              required_approving_review_count: 1,
              dismiss_stale_reviews_on_push: true,
              allowed_merge_methods: ['squash'],
            },
            required_status_checks: {
              required_status_checks: [{ context: 'test' }, { context: 'lint' }],
              strict_required_status_checks_policy: true,
            },
          },
        },
        {
          name: 'release-tags',
          target: 'tag',
          enforcement: 'evaluate',
          conditions: { include: ['refs/tags/v*'], exclude: [] },
          rules: { deletion: true, non_fast_forward: true },
        },
      ],
      allow_removal: false,
      excludes: ['hand-made-*'],
    },
    unreadable: false,
    unavailable: '',
  };

  const PLAN: SyncPlan = {
    id: 'plan-1',
    trigger: 'sweep',
    state: 'computed',
    execution_stage: 'Waiting for approval',
    digest: 'digest',
    counts: { create: 1, update: 0, delete: 0 },
    actions: [
      {
        repository: 'api-gateway',
        kind: 'rulesets',
        operation: 'create',
        subject: 'main-protection',
        after: '6 rules, active',
        state: 'pending',
      },
    ],
    computed_at: at(-12 * 60_000),
    expires_at: at(6 * 3_600_000),
  };

  const { Story } = defineMeta({
    title: 'Views/SyncRulesetsPage',
    component: SyncRulesetsPage,
    args: {
      config: CONFIG,
      plan: PLAN,
      readOnly: false,
      problem: null,
      syncStatus: SYNC_STATUS,
      nowMs: NOW,
      rulesetHref: (name: string) => `#/sync/rulesets/${name}`,
      onOpenRuleset: fn(),
      onToggleEnabled: fn(),
      onChangeDocument: fn(),
    },
  });
</script>

<!--
  A list of named objects, two levels deep and no deeper - press a row for
  the ruleset's own page. Enforcement is worn as a pill on the row, so
  Evaluate mode is visible from the list, and the plan's verdict rides the
  row's end.
-->
<Story name="Two rulesets" />

<Story name="Unsaved rulesets" args={{ dirtyDocument: true, savedDocument: {} }} />

<!-- Nothing named yet. -->
<Story
  name="Empty"
  args={{ config: { ...CONFIG, document: { rulesets: [], allow_removal: false, excludes: [] } } }}
/>

<!-- A reader without write. -->
<Story name="Read only" args={{ readOnly: true }} />

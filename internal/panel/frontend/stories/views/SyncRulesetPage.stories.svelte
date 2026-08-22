<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SyncRulesetPage from '#lib/components/SyncRulesetPage.svelte';
  import type { SyncConfig } from '#lib/types.js';

  /* The approved mock's detail: main-protection on the default branch, six
     rules on, two bypass actors. */
  const CONFIG: SyncConfig = {
    kind: 'rulesets',
    enabled: true,
    labels: [],
    allow_removal: false,
    excludes: [],
    revision: 2,
    updated_by: 'bart',
    updated_at: new Date(Date.UTC(2026, 7, 15)).toISOString(),
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
      ],
      allow_removal: false,
      excludes: [],
    },
    unreadable: false,
    unavailable: '',
  };

  const { Story } = defineMeta({
    title: 'Views/SyncRulesetPage',
    component: SyncRulesetPage,
    args: {
      config: CONFIG,
      name: 'main-protection',
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
  One ruleset's own page: enforcement as a three-state seg with its
  consequence said underneath, coverage as chips, every rule a settings row
  whose parameters ARE its value, and the actors who may step around it.
  Deleting waits at the bottom, on the sticky bar.
-->
<Story name="main-protection" />

<!-- An address naming a ruleset that is gone says so rather than rendering
     an empty editor. -->
<Story name="Not found" args={{ name: 'renamed-away' }} />

<!-- A reader without write: everything reads, nothing answers. -->
<Story name="Read only" args={{ readOnly: true }} />

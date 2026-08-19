<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SyncRulesetDetail from '#lib/components/SyncRulesetDetail.svelte';

  const STORED = {
    rulesets: [
      {
        name: 'main-branch-protection',
        target: 'branch',
        enforcement: 'active',
        conditions: { include: ['~DEFAULT_BRANCH'], exclude: [] },
        rules: {
          deletion: true,
          non_fast_forward: true,
          required_linear_history: true,
          required_signatures: true,
          pull_request: {
            required_approving_review_count: 1,
            require_code_owner_review: true,
            dismiss_stale_reviews_on_push: true,
            allowed_merge_methods: ['squash'],
          },
          required_status_checks: {
            required_status_checks: [{ context: 'test' }, { context: 'lint' }],
            strict_required_status_checks_policy: true,
          },
        },
        bypass_actors: [
          { actor_id: 0, actor_type: 'OrganizationAdmin', bypass_mode: 'always' },
          { actor_id: 41231, actor_type: 'Integration', bypass_mode: 'pull_request' },
        ],
      },
    ],
  };

  const { Story } = defineMeta({
    title: 'Views/SyncRulesetDetail',
    component: SyncRulesetDetail,
    args: {
      stored: STORED,
      name: 'main-branch-protection',
      listHref: '#rulesets',
      readOnly: false,
      saving: false,
      unreadable: false,
      onSave: fn(),
    },
  });
</script>

<!--
  The settings page's language one level down: only the rules that are ON are
  rows, each carrying its parameters as quiet chips, and the rest is one
  sentence naming them. Enforcement stays radio cards - three modes, one
  expensive wrong pick, and Evaluate is a ruleset that looks enforced and
  enforces nothing.
-->
<Story name="An enforced ruleset" />

<!-- A dry run. The pill and the chosen card say the same thing twice. -->
<Story
  name="Evaluating"
  args={{
    stored: {
      rulesets: [{ ...STORED.rulesets[0], enforcement: 'evaluate' }],
    },
  }}
/>

<!-- Nothing enforced yet, so the whole card is the sentence - which is the
     honest shape of a ruleset that would be written and hold nobody to
     anything. -->
<Story
  name="Nothing enforced"
  args={{
    stored: {
      rulesets: [{ ...STORED.rulesets[0], rules: {}, bypass_actors: [] }],
    },
  }}
/>

<!-- An address written down before the ruleset was renamed. The page says so
     rather than drawing an empty form somebody could save over. -->
<Story name="No ruleset by that name" args={{ name: 'archived-protection' }} />

<Story name="Read only" args={{ readOnly: true }} />

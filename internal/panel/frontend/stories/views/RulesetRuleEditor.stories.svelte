<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import RulesetRuleEditorStory from '../support/RulesetRuleEditorStory.svelte';
  const { Story } = defineMeta({
    title: 'Views/RulesetRuleEditor',
    component: RulesetRuleEditorStory,
  });
</script>

<Story name="Pull request" />
<Story name="Pinned required check" args={{ ruleKey: 'required_status_checks' }} />
<Story name="Upstream updates" args={{ ruleKey: 'update' }} />
<Story name="Code scanning thresholds" args={{ ruleKey: 'code_scanning' }} />
<Story name="New check rule" args={{ ruleKey: 'required_status_checks', fresh: true }} />
<Story name="Read only" args={{ readOnly: true }} />

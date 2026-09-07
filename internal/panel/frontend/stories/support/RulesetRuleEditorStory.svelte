<script lang="ts">
  import { onMount } from 'svelte';
  import RulesetRuleEditor from '#lib/components/RulesetRuleEditor.svelte';
  import Button from '#lib/components/Button.svelte';
  import { parseJson } from '#lib/merge.js';
  import type { SyncRulesetRules } from '#lib/types.js';
  import type { EditableRuleKey } from '../../src/lib/ruleset-rule-editor';

  const {
    ruleKey = 'pull_request',
    readOnly = false,
    fresh = false,
  }: { ruleKey?: EditableRuleKey; readOnly?: boolean; fresh?: boolean } = $props();
  let editor: { show: (key: EditableRuleKey, element?: HTMLElement) => void } | undefined =
    $state();
  let locked = $state(false);
  const rules = $derived(
    fresh
      ? {}
      : (parseJson(
          '{"pull_request":{"required_approving_review_count":2,"allowed_merge_methods":["squash"],"dismiss_stale_reviews_on_push":true},"required_status_checks":{"required_status_checks":[{"context":"test","integration_id":9007199254740993}],"strict_required_status_checks_policy":true},"update":{"update_allows_fetch_and_merge":true},"code_scanning":{"code_scanning_tools":[{"tool":"CodeQL","alerts_threshold":"errors","security_alerts_threshold":"high_or_higher"}]}}',
        ) as SyncRulesetRules),
  );
  onMount(() => {
    editor?.show(ruleKey);
    locked = readOnly;
  });
</script>

<Button
  onclick={(event) => {
    locked = false;
    editor?.show(ruleKey, event.currentTarget);
    locked = readOnly;
  }}>Edit rule</Button
>
<RulesetRuleEditor
  bind:this={editor}
  scope="story/main-protection"
  rulesetName="main-protection"
  {rules}
  disabled={locked}
  onApply={() => {}}
/>

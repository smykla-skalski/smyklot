<script lang="ts">
  import type { BypassActorLookup, PendingCIBypassPolicy } from '../types';
  import BypassActorEditor from './BypassActorEditor.svelte';
  import Select from './Select.svelte';

  const {
    value,
    inherited,
    readOnly = false,
    organizationActors = true,
    lookup,
    onChange,
  }: {
    value: PendingCIBypassPolicy | null;
    inherited?: PendingCIBypassPolicy | null;
    readOnly?: boolean;
    organizationActors?: boolean;
    lookup?: BypassActorLookup;
    onChange: (value: PendingCIBypassPolicy | null) => void;
  } = $props();

  const repository = $derived(inherited !== undefined);
  const effective = $derived(value ?? inherited ?? null);
  const selection = $derived(value === null ? 'inherit' : value.allow ? 'allow' : 'deny');
  const options = $derived([
    { value: 'inherit', label: repository ? 'Follow workspace' : 'Keep existing exceptions' },
    { value: 'deny', label: 'No exceptions' },
    { value: 'allow', label: 'Allow selected actors' },
  ]);

  const explanation = $derived(
    value === null
      ? repository
        ? `Workspace default: ${effective === null ? 'keep existing exceptions' : effective.allow ? 'selected actors' : 'no exceptions'}`
        : 'Existing lists stay in place, new lists start empty'
      : value.allow
        ? 'Selected actors may bypass merge-after-CI'
        : value.actors.length > 0
          ? 'No bypasses, your actor list is kept for later'
          : 'Every actor must pass merge-after-CI',
  );

  function changePolicy(selection: string): void {
    if (readOnly) return;
    onChange(
      selection === 'inherit'
        ? null
        : { allow: selection === 'allow', actors: effective?.actors ?? [] },
    );
  }
</script>

<!--
@component
Ownership of merge-after-CI exceptions, above the shared actor editor. A null
workspace answer preserves existing GitHub exceptions, while a null repository
answer inherits its workspace. Denying exceptions keeps the selected actors in
the saved policy so re-enabling it does not require finding everybody again.
Inherited actors stay read only until the repository chooses its own policy.
-->

<div class="policy-rows" class:rows-continue={effective?.allow}>
  <div class="policy-row">
    <span class="setting-say"
      ><span class="setting-name">Who may bypass</span><span class="setting-why">{explanation}</span
      ></span
    >
    <span class="policy-value"
      ><Select
        aria-label="Bypass exception policy"
        value={selection}
        {options}
        disabled={readOnly}
        onValueChange={changePolicy}
      /></span
    >
  </div>
</div>

{#if effective?.allow}
  <BypassActorEditor
    {organizationActors}
    actors={effective.actors}
    {lookup}
    readOnly={readOnly || (repository && value === null)}
    onChange={(actors) => onChange({ allow: true, actors })}
  />
{/if}

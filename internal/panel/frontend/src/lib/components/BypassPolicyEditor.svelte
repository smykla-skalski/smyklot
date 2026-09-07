<script lang="ts">
  import type { BypassActorLookup, PendingCIBypassPolicy } from '../types';
  import BypassActorEditor from './BypassActorEditor.svelte';
  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import Icon from './Icon.svelte';
  import Select from './Select.svelte';

  const {
    value,
    inherited,
    readOnly = false,
    organizationActors = true,
    lookup,
    onChange,
    id,
    unsaved = false,
  }: {
    value: PendingCIBypassPolicy | null;
    inherited?: PendingCIBypassPolicy | null;
    readOnly?: boolean;
    organizationActors?: boolean;
    lookup?: BypassActorLookup;
    onChange: (value: PendingCIBypassPolicy | null) => void;
    id?: string;
    unsaved?: boolean;
  } = $props();

  const repository = $derived(inherited !== undefined);
  let actorEditor: BypassActorEditor | undefined = $state();
  let adding = $state(false);
  const effective = $derived(value ?? inherited ?? null);
  const actorsReadOnly = $derived(readOnly || (repository && value === null));
  const selection = $derived(value === null ? 'inherit' : value.allow ? 'allow' : 'deny');
  const options = $derived([
    { value: 'inherit', label: repository ? 'Follow workspace' : 'Keep GitHub exceptions' },
    { value: 'deny', label: 'No exceptions' },
    { value: 'allow', label: 'Allow listed actors' },
  ]);

  const explanation = $derived(
    value === null
      ? repository
        ? `Workspace default: ${effective === null ? 'keep each repository’s GitHub exceptions' : effective.allow ? 'allow the actors listed below' : 'no exceptions'}`
        : 'Keep each repository’s existing GitHub exceptions'
      : value.allow
        ? value.actors.length > 0
          ? 'Only the actors listed below may bypass merge-after-CI'
          : 'No actors may bypass merge-after-CI until one is added'
        : value.actors.length > 0
          ? 'No bypasses, your actor list is kept for later'
          : 'Every actor must pass merge-after-CI',
  );

  function changePolicy(selection: string): void {
    if (readOnly) return;
    adding = false;
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

<Card {id} {unsaved} label="Merge exceptions">
  <div class="card-head">
    <h2 class="card-title">Merge exceptions</h2>
    {#if effective?.allow && !actorsReadOnly}
      <Button
        tone="add"
        aria-expanded={adding}
        onclick={(event) => actorEditor?.toggleAdd(event.currentTarget)}
      >
        {#snippet icon()}<Icon name="plus" size="sm" />{/snippet}
        Add an actor
      </Button>
    {/if}
  </div>
  <div class="policy-rows" class:rows-continue={effective?.allow}>
    <div class="policy-row">
      <span class="setting-say"
        ><span class="setting-name">Bypass policy</span><span class="setting-why"
          >{explanation}</span
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
      bind:this={actorEditor}
      bind:adding
      showAddButton={false}
      {organizationActors}
      actors={effective.actors}
      {lookup}
      readOnly={actorsReadOnly}
      onChange={(actors) => onChange({ allow: true, actors })}
    />
  {/if}
</Card>

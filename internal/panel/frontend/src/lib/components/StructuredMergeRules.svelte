<script lang="ts">
  import { patchedAt, withoutAt } from '#lib/form-lists.js';
  import { asArrayStrategy } from '#lib/merge.js';
  import type { SyncFileMerge } from '#lib/types.js';

  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import Icon from './Icon.svelte';
  import IconButton from './IconButton.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import Switch from './Switch.svelte';

  const {
    merge,
    idPrefix,
    disabled = false,
    readOnly = false,
    onChange,
  }: {
    merge: SyncFileMerge;
    idPrefix: string;
    disabled?: boolean;
    readOnly?: boolean;
    onChange: (change: Partial<SyncFileMerge>) => void;
  } = $props();

  const OBJECT_CHOICES = [
    { value: '', label: 'Default' },
    { value: 'deep-merge', label: 'Merge' },
    { value: 'shallow-merge', label: 'Replace' },
  ];
  const LIST_CHOICES = [
    { value: 'append', label: 'Append' },
    { value: 'prepend', label: 'Prepend' },
    { value: 'replace', label: 'Replace' },
  ];
  const rules = $derived(merge.arrays ?? []);
  const held = $derived(disabled || readOnly);

  function listExplanation(strategy: string): string {
    if (strategy === 'append') return 'Add repository entries after the template entries';
    if (strategy === 'prepend') return 'Add repository entries before the template entries';
    return 'Use only the repository entries';
  }

  function changeStrategy(index: number, selection: string): void {
    const strategy = asArrayStrategy(selection);
    if (strategy !== undefined) {
      onChange({ arrays: patchedAt(rules, index, { strategy }) });
    }
  }
</script>

<!--
@component
The rare decisions about combining a structured template and repository content.
The parent owns the draft and validation, so leaving this inspector keeps unfinished
rules exactly as they were typed and never changes the mounted content editor.
-->

<div class="card-stack">
  <Card label="Object rules">
    <div class="card-head"><h3 class="card-title">Objects</h3></div>
    <div class="policy-rows">
      <div class="policy-row">
        <span class="setting-say">
          <span class="setting-name">Nested values</span>
          <span class="setting-why">
            {merge.strategy === 'shallow-merge'
              ? 'Replace each adjusted top-level value, including its nested keys'
              : 'Merge nested keys and keep template values you have not changed'}
          </span>
        </span>
        <span class="policy-value">
          <SegmentedControl
            name={`${idPrefix}-object-strategy`}
            label={`How ${merge.path || 'this file'} is composed`}
            compact
            options={OBJECT_CHOICES}
            value={merge.strategy ?? ''}
            disabled={held}
            onSelect={(strategy) => onChange({ strategy })}
          />
        </span>
      </div>
    </div>
  </Card>

  <Card label="List rules">
    <div class="card-head">
      <h3 class="card-title">Lists</h3>
      {#if !readOnly}
        <Button
          tone="quiet"
          disabled={held}
          onclick={() => onChange({ arrays: [...rules, { path: '', strategy: 'append' }] })}
        >
          {#snippet icon()}<Icon name="plus" size="sm" />{/snippet}Add a list rule
        </Button>
      {/if}
    </div>
    <p class="group-note">Lists replace the template unless a rule below says otherwise</p>
    {#if rules.length > 0}
      <div class="policy-rows">
        {#each rules as rule, index (`${idPrefix}-rule-${index}`)}
          <div class="policy-row">
            <label class="setting-say" for={`${idPrefix}-list-${index}`}>
              <span class="setting-name" id={`${idPrefix}-list-${index}-name`}>List</span>
              <span class="setting-why">A list set in your content adjustments</span>
            </label>
            <span class="policy-value list-path-controls">
              <input
                id={`${idPrefix}-list-${index}`}
                aria-labelledby={`${idPrefix}-list-${index}-name`}
                class="text-input list-path"
                type="text"
                value={rule.path}
                disabled={held}
                placeholder="$.packageRules"
                oninput={(event) =>
                  onChange({
                    arrays: patchedAt(rules, index, { path: event.currentTarget.value }),
                  })}
              />
              {#if !readOnly}
                <IconButton
                  toolbar
                  icon="trash"
                  label={`Remove list rule ${rule.path || 'without a path'}`}
                  disabled={held}
                  onclick={() => onChange({ arrays: withoutAt(rules, index) })}
                />
              {/if}
            </span>
          </div>
          <div class="policy-row">
            <span class="setting-say">
              <span class="setting-name">Combine entries</span>
              <span class="setting-why">{listExplanation(rule.strategy)}</span>
            </span>
            <span class="policy-value">
              <SegmentedControl
                name={`${idPrefix}-list-strategy-${index}`}
                label={`What happens to ${rule.path || 'this list'}`}
                compact
                options={LIST_CHOICES}
                value={rule.strategy}
                disabled={held}
                onSelect={(selection) => changeStrategy(index, selection)}
              />
            </span>
          </div>
        {/each}
        <div class="policy-row">
          <span class="setting-say">
            <span class="setting-name">Drop repeated entries</span>
            <span class="setting-why">Keep one copy when a list rule combines matching entries</span
            >
          </span>
          <span class="policy-value">
            <Switch
              checked={merge.deduplicate === true}
              bare
              label={`Drop repeated entries from ${merge.path || 'this file'}`}
              disabled={held}
              onToggle={(next) => onChange({ deduplicate: next ? true : undefined })}
            />
          </span>
        </div>
      </div>
    {/if}
  </Card>
</div>

<style>
  .list-path-controls {
    min-inline-size: 0;
  }
  .list-path {
    flex: 1 1 10rem;
    font-family: var(--mono);
    inline-size: 10rem;
    min-inline-size: 0;
    max-inline-size: 100%;
  }
</style>

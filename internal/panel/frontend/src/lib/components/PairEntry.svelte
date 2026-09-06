<script module lang="ts">
  export interface PairOption {
    value: string;
    label: string;
    description?: string;
  }
</script>

<script lang="ts">
  import { Combobox } from 'bits-ui';
  import { flushSync, untrack } from 'svelte';

  import Icon from './Icon.svelte';

  const {
    keyValue,
    value,
    keyLabel,
    valueLabel,
    removeLabel,
    options,
    disabled = false,
    draft = false,
    focusOnMount = false,
    validateKey,
    onCommit,
    onRemove,
    onProblem = () => {},
  }: {
    keyValue: string;
    value: string;
    keyLabel: string;
    valueLabel: string;
    removeLabel: string;
    options: readonly PairOption[];
    disabled?: boolean;
    draft?: boolean;
    focusOnMount?: boolean;
    validateKey: (key: string) => string | null;
    onCommit: (key: string, value: string) => void;
    onRemove: () => void;
    onProblem?: (problem: string | null) => void;
  } = $props();

  const instanceId = $props.id();
  let keyText = $state(untrack(() => keyValue));
  let keyField = $state<HTMLInputElement | null>(null);
  let selected = $state(untrack(() => value));
  let query = $state<string | null>(null);
  let open = $state(false);
  let touched = $state(false);
  const problem = $derived(touched ? validateKey(keyText.trim()) : null);
  const shownValue = $derived(
    query ?? options.find((option) => option.value === selected)?.label ?? selected,
  );
  const matches = $derived(
    options.filter(
      (option) =>
        query === null ||
        `${option.label} ${option.description ?? ''}`.toLowerCase().includes(query.toLowerCase()),
    ),
  );

  // External reset restores both halves. A valid commit is adopted by the owner.
  $effect(() => {
    keyText = keyValue;
    selected = value;
    touched = false;
    query = null;
  });
  $effect(() => onProblem(problem));
  $effect(() => {
    if (focusOnMount) untrack(() => queueMicrotask(() => keyField?.focus()));
  });

  function commit(nextValue = selected): void {
    touched = true;
    const key = keyText.trim();
    if (validateKey(key) !== null || !options.some((option) => option.value === nextValue)) return;
    keyText = key;
    if (key !== keyValue || nextValue !== value) onCommit(key, nextValue);
  }

  function choose(next: string): void {
    if (!options.some((option) => option.value === next)) return;
    selected = next;
    query = null;
    commit(next);
  }

  function close(next: boolean): void {
    open = next;
    // Typing searches the known set; only choosing an option changes the value.
    if (!next) query = null;
  }
</script>

<!--
@component
A key and a selected value share one lane. Each half owns its focus state;
the inputs are transparent and never supply a second border or background.
The value half uses the same Bits combobox primitive as account suggestions.
-->

<span class="pair-entry" class:is-draft={draft} aria-disabled={disabled || undefined}>
  <span class="pair-seg is-key" class:is-invalid={problem !== null}>
    <span class="pair-fit">
      <span class="pair-ghost" aria-hidden="true">{keyText || 'alias'}</span>
      <input
        bind:this={keyField}
        class="pair-input"
        value={keyText}
        aria-label={keyLabel}
        aria-invalid={problem !== null || undefined}
        aria-describedby={problem === null ? undefined : `${instanceId}-problem`}
        placeholder="alias"
        autocomplete="off"
        spellcheck="false"
        {disabled}
        oninput={(event) => (keyText = event.currentTarget.value)}
        onblur={() => commit()}
        onkeydown={(event) => {
          if (event.key === 'Enter') {
            event.preventDefault();
            commit();
          }
          if (event.key === 'Escape') {
            keyText = keyValue;
            touched = false;
          }
        }}
      />
    </span>
  </span>
  <span class="pair-arrow" aria-hidden="true">→</span>
  <Combobox.Root
    type="single"
    allowDeselect={false}
    items={options.map((option) => ({ value: option.value, label: option.label }))}
    value={selected}
    inputValue={shownValue}
    {open}
    {disabled}
    onValueChange={choose}
    onOpenChange={close}
  >
    <span class="pair-seg is-value">
      <span class="pair-fit">
        <span class="pair-ghost" aria-hidden="true">{shownValue || 'command'}</span>
        <Combobox.Input
          class="pair-input pair-value-input"
          aria-label={valueLabel}
          placeholder="command"
          autocomplete="off"
          spellcheck="false"
          onfocus={() => {
            query = null;
            open = true;
          }}
          onclick={() => {
            if (open) return;
            query = null;
            open = true;
          }}
          oninput={(event) => {
            const nextQuery = event.currentTarget.value;
            // Bits highlights a result in this same input event. Publish the filtered
            // rows first so its active descendant cannot point to a removed option.
            flushSync(() => {
              query = nextQuery;
              open = true;
            });
          }}
        />
      </span>
      <Combobox.Trigger class="pair-open" aria-label="Show commands" tabindex={-1}>
        <Icon name="chevron-down" size="xs" />
      </Combobox.Trigger>
    </span>
    <Combobox.Portal to=".app-shell">
      <Combobox.Content class="pair-menu" sideOffset={4} collisionPadding={8}>
        <Combobox.Viewport class="menu-list" aria-label={valueLabel}>
          {#each matches as option (option.value)}
            <Combobox.Item class="pair-option" value={option.value} label={option.label}>
              <span class="pair-option-name">{option.label}</span>
              <span class="pair-option-description">{option.description ?? ''}</span>
              <span class="pair-option-check">
                {#if selected === option.value}<Icon name="check" size="xs" />{/if}
              </span>
            </Combobox.Item>
          {:else}
            <span class="pair-empty" role="status">No matching command</span>
          {/each}
        </Combobox.Viewport>
      </Combobox.Content>
    </Combobox.Portal>
  </Combobox.Root>
  <button class="pair-del" type="button" aria-label={removeLabel} {disabled} onclick={onRemove}>
    <Icon name="close" size="xs" />
  </button>
</span>
{#if problem !== null}
  <span class="pair-problem" id="{instanceId}-problem" role="alert">{problem}</span>
{/if}

<style>
  .pair-entry {
    --pair-air: var(--space-1);
    --pair-lane-height: calc(var(--control-height-compact) - 4px);
    --pair-lane-radius: calc(var(--r-ctl) - 1px);
    align-items: center;
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    display: inline-grid;
    flex: none;
    grid-template-columns: minmax(0, max-content) auto minmax(0, max-content) calc(
        var(--pair-lane-height) + var(--pair-air)
      );
    min-block-size: var(--control-height-compact);
    max-inline-size: 100%;
    padding-inline: var(--pair-air);
  }
  .pair-entry:hover:not([aria-disabled='true']) {
    border-color: var(--control-border-hover);
  }
  .pair-entry.is-draft {
    background: transparent;
    border-style: dashed;
  }
  .pair-entry[aria-disabled='true'] {
    opacity: var(--disabled-opacity, 0.45);
  }
  .pair-seg {
    align-items: stretch;
    block-size: var(--pair-lane-height);
    border-radius: var(--pair-lane-radius);
    display: grid;
    gap: var(--pair-air);
    grid-template-columns: minmax(0, auto) auto;
    min-inline-size: 0;
    padding-inline: var(--pair-air);
  }
  .is-value {
    padding-inline-end: 0;
  }
  .pair-fit {
    display: grid;
    min-inline-size: var(--field-target-min);
    position: relative;
  }
  .is-key .pair-fit {
    max-inline-size: 7.5rem;
  }
  .is-value .pair-fit {
    max-inline-size: 9.5rem;
  }
  .pair-ghost {
    align-self: center;
    font: var(--font-size-compact) / var(--leading-flat) var(--mono);
    grid-area: 1 / 1;
    overflow: hidden;
    padding-inline-end: 1px;
    pointer-events: none;
    visibility: hidden;
    white-space: pre;
  }
  .pair-entry :global(.pair-input) {
    background: transparent;
    block-size: calc(100% + 2px);
    border: 0;
    border-radius: 0;
    box-shadow: none;
    color: var(--text-primary);
    font: var(--font-size-compact) / var(--leading-flat) var(--mono);
    inline-size: calc(100% + var(--pair-air));
    inset-block: -1px;
    inset-inline-start: calc(var(--pair-air) * -1);
    min-inline-size: 0;
    outline: none;
    padding: 0 0 0 var(--pair-air);
    position: absolute;
    text-overflow: ellipsis;
  }
  .is-key .pair-input {
    inline-size: calc(100% + 2 * var(--pair-air));
    inset-inline-start: calc(var(--pair-air) * -2);
    padding-inline-start: calc(2 * var(--pair-air));
  }
  .pair-entry :global(.pair-value-input) {
    color: var(--code-const);
  }
  .pair-entry :global(.pair-input::placeholder) {
    color: var(--text-muted);
  }
  .pair-arrow {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    padding-inline: var(--space-2);
    text-box: trim-both cap alphabetic;
  }
  .pair-seg:focus-within {
    background: color-mix(in srgb, var(--focus) 9%, transparent);
    outline: var(--focus-ring-width) solid var(--focus);
    outline-offset: var(--focus-ring-inset);
  }
  .pair-seg.is-invalid:not(:focus-within) {
    background: var(--danger-tint);
    outline: var(--focus-ring-width) solid var(--danger);
    outline-offset: var(--focus-ring-inset);
  }
  .pair-entry:not([aria-disabled='true'])
    .pair-seg.is-value:has(:global(:hover)):not(:has(:global(:active))) {
    background: var(--interactive-hover-layer);
  }
  .pair-entry:not([aria-disabled='true']) .pair-seg.is-value:has(:global(.pair-open:active)) {
    background: var(--interactive-pressed);
    box-shadow: var(--pressed-inset);
  }
  .pair-entry :global(.pair-open),
  .pair-del {
    align-items: center;
    background: transparent;
    block-size: var(--pair-lane-height);
    border: 0;
    border-radius: var(--pair-lane-radius);
    color: var(--text-muted);
    cursor: pointer;
    display: flex;
    inline-size: var(--pair-lane-height);
    justify-content: center;
    padding: 0;
  }
  .pair-entry :global(.pair-open[aria-expanded='true'] svg) {
    rotate: 180deg;
  }
  .pair-del {
    justify-self: end;
  }
  .pair-del:hover:not(:active):not(:disabled) {
    background: var(--danger-tint);
    color: var(--danger);
  }
  .pair-del:active:not(:disabled) {
    background: var(--danger-tint);
    box-shadow: var(--pressed-inset);
    color: var(--danger);
  }
  .pair-entry :global(.pair-open:active),
  .pair-del:active {
    translate: none;
  }
  .pair-entry :global(.pair-open:active > *),
  .pair-del:active > :global(*) {
    translate: none;
  }
  .pair-problem {
    color: var(--danger);
    flex-basis: 100%;
    font-size: var(--font-size-compact);
  }
  :global(.pair-menu) {
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: var(--radius-popover);
    box-shadow: var(--shadow-popover);
    max-height: var(--bits-floating-available-height);
    max-width: min(25rem, var(--bits-floating-available-width));
    overflow: auto;
    z-index: var(--layer-popover);
  }
  :global(.pair-option) {
    align-items: center;
    border-radius: var(--r-ctl);
    cursor: pointer;
    display: flex;
    gap: var(--space-3);
    min-block-size: var(--control-height-compact);
    padding: 0 var(--space-3);
  }
  :global(.pair-option[data-highlighted]) {
    background: var(--interactive-hover-layer);
  }
  .pair-option-name {
    font: var(--font-size-compact) / var(--leading-flat) var(--mono);
  }
  .pair-option-description {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    line-height: var(--leading-compact);
    margin-inline-start: auto;
  }
  .pair-option-check {
    display: grid;
    inline-size: 14px;
    place-items: center;
  }
  .pair-empty {
    color: var(--text-muted);
    display: block;
    font-size: var(--font-size-compact);
    padding: var(--space-3);
  }
</style>

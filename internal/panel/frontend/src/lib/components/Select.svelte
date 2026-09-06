<script module lang="ts">
  export type SelectValue = string | number | null | undefined;
  export type SelectOption<Value extends SelectValue = SelectValue> = {
    value: Value;
    label: string;
    disabled?: boolean;
  };
</script>

<script lang="ts" generics="Value extends SelectValue">
  import { Select as Listbox } from 'bits-ui';
  import { untrack } from 'svelte';
  import type { HTMLButtonAttributes } from 'svelte/elements';

  import Icon from './Icon.svelte';
  import PickerTrigger from './PickerTrigger.svelte';

  let {
    value = $bindable(),
    options,
    onValueChange,
    placeholder = 'Choose an option',
    disabled = false,
    required = false,
    name,
    form,
    id,
    class: extra = '',
    ...rest
  }: {
    value: Value;
    options: readonly SelectOption<Value>[];
    onValueChange?: (value: Value) => void;
    placeholder?: string;
    disabled?: boolean;
    required?: boolean;
    name?: string;
    form?: string;
    id?: string;
    class?: string;
  } & Omit<
    HTMLButtonAttributes,
    'value' | 'onchange' | 'oninput' | 'name' | 'form' | 'type' | 'class' | 'children'
  > = $props();

  const originalValue = untrack(() => value);
  const menuId = $props.id();
  let open = $state(false);
  let invalid = $state(false);
  let trigger = $state<HTMLButtonElement | null>(null);

  // Bits UI uses string keys. Keep that implementation detail out of the public
  // value and native form value, including distinct null, empty and numeric values.
  function keyFor(candidate: SelectValue): string {
    if (candidate === undefined) return '';
    if (Object.is(candidate, -0)) return 'number:-0';
    return JSON.stringify([typeof candidate, candidate]);
  }

  const entries = $derived(options.map((option) => ({ ...option, key: keyFor(option.value) })));
  const selected = $derived(entries.find((option) => Object.is(option.value, value)));
  const label = $derived(selected?.label ?? (value == null ? placeholder : String(value)));
  const formValue = $derived(value == null ? '' : String(value));

  function choose(key: string): void {
    const option = entries.find((entry) => entry.key === key);
    if (!option || option.disabled) return;
    invalid = false;
    value = option.value as Value;
    onValueChange?.(value);
  }

  function connectForm(element: HTMLSpanElement): (() => void) | undefined {
    const owner = form ? document.getElementById(form) : element.closest('form');
    if (!(owner instanceof HTMLFormElement)) return;
    function reset(event: Event): void {
      // A later form listener may cancel the reset. Match the native control's
      // default-value behavior without resetting the owner's other fields.
      queueMicrotask(() => {
        if (event.defaultPrevented) return;
        value = originalValue;
        invalid = false;
        open = false;
        onValueChange?.(value);
      });
    }
    owner.addEventListener('reset', reset);
    return () => owner.removeEventListener('reset', reset);
  }
</script>

<!--
@component
An intrinsic shared picker with a fully themed option menu. Values retain their
original types through bindings and callbacks; internal menu keys never enter form
data. Bits UI owns selection, keyboard navigation, typeahead and focus behavior.
A form-backed field preserves submission, required validation and reset behavior.
-->

<span class="select-wrap" {@attach connectForm}>
  <Listbox.Root
    type="single"
    value={keyFor(value)}
    items={entries.map((option) => ({
      value: option.key,
      label: option.label,
      disabled: option.disabled,
    }))}
    {disabled}
    {required}
    bind:open
    onValueChange={choose}
  >
    <Listbox.Trigger
      {...rest}
      {id}
      bind:ref={trigger}
      role="combobox"
      aria-controls={open ? menuId : undefined}
      aria-required={required || undefined}
      aria-invalid={invalid || rest['aria-invalid']}
    >
      {#snippet child({ props })}
        <PickerTrigger {...props}><span class={extra}>{label}</span></PickerTrigger>
      {/snippet}
    </Listbox.Trigger>
    <Listbox.Portal
      to={typeof document === 'undefined'
        ? undefined
        : (document.querySelector('.app-shell') ?? undefined)}
    >
      <Listbox.Content
        id={menuId}
        class={['select-menu', trigger?.closest('[role="dialog"]') && 'select-menu-in-dialog']}
        strategy="fixed"
        sideOffset={4}
        align="start"
        collisionPadding={8}
        aria-label={rest['aria-label']}
        aria-labelledby={!rest['aria-label'] ? trigger?.id : undefined}
      >
        <Listbox.Viewport class="menu-list select-options">
          {#each entries as option (option.key)}
            <Listbox.Item
              class="menu-option"
              value={option.key}
              label={option.label}
              disabled={option.disabled}
            >
              <span class="menu-option-check" aria-hidden="true">
                {#if Object.is(option.value, value)}<Icon name="check" size="base" />{/if}
              </span>
              <span class={['mi-label', extra]}>{option.label}</span>
            </Listbox.Item>
          {/each}
        </Listbox.Viewport>
      </Listbox.Content>
    </Listbox.Portal>
  </Listbox.Root>
  {#if name || required}
    <input
      class="visually-hidden"
      tabindex="-1"
      aria-hidden="true"
      {name}
      {form}
      {disabled}
      {required}
      value={formValue}
      oninvalid={(event) => {
        event.preventDefault();
        invalid = true;
        trigger?.focus();
      }}
    />
  {/if}
</span>

<style>
  :global(.select-menu) {
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: var(--radius-popover);
    box-shadow: var(--shadow-popover);
    color: var(--text-primary);
    max-block-size: min(20rem, var(--bits-floating-available-height));
    max-inline-size: var(--bits-floating-available-width);
    min-inline-size: var(--bits-floating-anchor-width);
    overflow: auto;
    z-index: var(--layer-popover);
  }
  :global(.select-menu-in-dialog) {
    z-index: var(--layer-dialog-popover);
  }
  :global(.select-options) {
    min-inline-size: 0;
  }
</style>

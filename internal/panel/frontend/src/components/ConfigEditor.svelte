<script lang="ts">
  import { untrack } from 'svelte';

  import {
    BOOLEAN_FIELDS,
    clonePatch,
    commandIsAllowed,
    effectiveValue,
    fieldEnabled,
    fieldRawValue,
    patchesEqual,
    reconcilePatchDraft,
    setExplicitPatchValue,
    toggleAllowedCommand,
    updatePatchValue,
  } from '../lib/config';
  import type { BooleanField } from '../lib/config';
  import { COMMANDS } from '../lib/types';
  import type { ConfigKey, ConfigPatch, ConfigValues } from '../lib/types';
  import HelpTip from './HelpTip.svelte';
  import Icon from './Icon.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  const MODE_OPTIONS = [
    { value: 'default', label: 'Default' },
    { value: 'custom', label: 'Custom' },
  ] as const;

  const ALL_KEYS: readonly ConfigKey[] = [
    ...BOOLEAN_FIELDS.map((field) => field.key),
    'command_prefix',
    'allowed_commands',
    'command_aliases',
  ];

  const {
    patch,
    inherited,
    scope,
    idPrefix,
    disabled = false,
    section = 'all',
    onSave,
  }: {
    patch: ConfigPatch;
    inherited: ConfigValues;
    scope: 'target' | 'repository' | 'runtime';
    idPrefix: string;
    disabled?: boolean;
    section?: 'all' | 'behavior' | 'commands';
    onSave: (next: ConfigPatch) => Promise<void>;
  } = $props();

  const initialPatch = clonePatch(untrack(() => patch));
  let draft = $state<ConfigPatch>(initialPatch);
  let receivedPatch = $state<ConfigPatch>(clonePatch(initialPatch));
  let saving = $state(false);
  let aliasName = $state('');
  let aliasCommand = $state('approve');
  let composerOpen = $state(false);

  const editorDisabled = $derived(disabled || saving);
  const aliasEntries = $derived(
    Object.entries(effectiveValue(draft, inherited, 'command_aliases')),
  );
  const savedAliases = $derived(patch.command_aliases ?? inherited.command_aliases);
  const allowedList = $derived(effectiveValue(draft, inherited, 'allowed_commands'));
  const allowedCount = $derived(allowedList.length === 0 ? COMMANDS.length : allowedList.length);

  const changedKeys = $derived(ALL_KEYS.filter((key) => !singleKeyEqual(draft, patch, key)));
  const dirty = $derived(changedKeys.length > 0);

  $effect(() => {
    const incoming = clonePatch(patch);
    const nextDraft = reconcilePatchDraft(draft, receivedPatch, incoming);
    if (nextDraft === draft) return;
    receivedPatch = incoming;
    draft = nextDraft;
  });

  function singleKeyEqual(left: ConfigPatch, right: ConfigPatch, key: ConfigKey): boolean {
    const a = Object.hasOwn(left, key) ? ({ [key]: left[key] } as ConfigPatch) : {};
    const b = Object.hasOwn(right, key) ? ({ [key]: right[key] } as ConfigPatch) : {};
    return patchesEqual(a, b);
  }

  function hasOverride(key: ConfigKey): boolean {
    return Object.hasOwn(draft, key);
  }

  function keyChanged(key: ConfigKey): boolean {
    return changedKeys.includes(key);
  }

  function useDefault(key: ConfigKey): void {
    if (!hasOverride(key)) return;
    const next = { ...draft };
    delete next[key];
    draft = next;
  }

  function useCustom(key: ConfigKey): void {
    if (hasOverride(key)) return;
    draft = { ...draft, [key]: cloneValue(inherited[key]) };
  }

  function selectBoolean(field: BooleanField, selection: string): void {
    if (selection === 'default') {
      useDefault(field.key);
      return;
    }
    draft = setExplicitPatchValue(draft, field.key, fieldRawValue(field, selection === 'enabled'));
  }

  function booleanValue(field: BooleanField): string {
    if (!hasOverride(field.key)) return 'default';
    const raw = effectiveValue(draft, inherited, field.key);
    return fieldEnabled(field, raw) ? 'enabled' : 'disabled';
  }

  function booleanOptions(field: BooleanField) {
    const inheritedEnabled = fieldEnabled(field, inherited[field.key]);
    return [
      {
        value: 'default',
        label: 'Default',
        detail: {
          text: inheritedEnabled ? 'Enabled' : 'Disabled',
          tone: inheritedEnabled ? ('on' as const) : ('off' as const),
        },
      },
      { value: 'enabled', label: 'Enabled', tone: 'on' as const },
      { value: 'disabled', label: 'Disabled', tone: 'off' as const },
    ];
  }

  function setPrefix(value: string): void {
    draft = updatePatchValue(draft, inherited, 'command_prefix', value);
  }

  function toggleCommand(command: string): void {
    const current = effectiveValue(draft, inherited, 'allowed_commands');
    const next = toggleAllowedCommand(current, command, COMMANDS);
    draft = updatePatchValue(draft, inherited, 'allowed_commands', next);
  }

  function openComposer(): void {
    composerOpen = true;
  }

  function closeComposer(): void {
    composerOpen = false;
    aliasName = '';
  }

  function addAlias(): void {
    const name = aliasName.trim();
    if (name === '') return;
    const current = effectiveValue(draft, inherited, 'command_aliases');
    draft = updatePatchValue(draft, inherited, 'command_aliases', {
      ...current,
      [name]: aliasCommand,
    });
    closeComposer();
  }

  function removeAlias(name: string): void {
    const next = { ...effectiveValue(draft, inherited, 'command_aliases') };
    delete next[name];
    draft = updatePatchValue(draft, inherited, 'command_aliases', next);
  }

  function discard(): void {
    draft = clonePatch(patch);
    closeComposer();
  }

  async function save(): Promise<void> {
    if (editorDisabled || !dirty) return;
    saving = true;
    try {
      await onSave(clonePatch(draft));
    } finally {
      saving = false;
    }
  }

  function guardUnload(event: BeforeUnloadEvent): void {
    if (dirty) event.preventDefault();
  }

  function cloneValue<T>(value: T): T {
    return JSON.parse(JSON.stringify(value)) as T;
  }
</script>

<svelte:window onbeforeunload={guardUnload} />

<div class="config-editor">
  {#if section === 'all' || section === 'behavior'}
    <section class="editor-section" aria-labelledby="config-{scope}-{idPrefix}-behavior">
      {#if section === 'all'}
        <header class="group-heading">
          <h3 id="config-{scope}-{idPrefix}-behavior">Behavior</h3>
          <p>How Smyklot replies and which safeguards apply</p>
        </header>
      {:else}
        <h3 class="visually-hidden" id="config-{scope}-{idPrefix}-behavior">Behavior</h3>
      {/if}

      <div class="rows">
        {#each BOOLEAN_FIELDS as field (field.key)}
          {@const overridden = Object.hasOwn(patch, field.key)}
          {@const changed = keyChanged(field.key)}
          <div class="row" class:overridden class:changed>
            <span class="row-label">
              <span class="label-text">{field.label}</span>
              <HelpTip
                id="config-{scope}-{idPrefix}-{field.key}-tooltip"
                label="About {field.label.toLowerCase()}"
                text={field.help}
                align="start"
                compact
              />
            </span>
            <span class="visually-hidden" id="config-{scope}-{idPrefix}-{field.key}-help">
              {field.help}
            </span>
            <span class="row-spacer"></span>
            <span class="changed-tag">Unsaved</span>
            {#if hasOverride(field.key) && !editorDisabled}
              <button class="reset-link" type="button" onclick={() => useDefault(field.key)}>
                Reset to default
              </button>
            {/if}
            <SegmentedControl
              name="config-{scope}-{idPrefix}-{field.key}"
              label={field.label}
              descriptionId="config-{scope}-{idPrefix}-{field.key}-help"
              options={booleanOptions(field)}
              value={booleanValue(field)}
              compact
              onSelect={(selection) => selectBoolean(field, selection)}
              disabled={editorDisabled}
            />
          </div>
        {/each}
      </div>
    </section>
  {/if}

  {#if section === 'all' || section === 'commands'}
    <section class="editor-section" aria-labelledby="config-{scope}-{idPrefix}-commands">
      {#if section === 'all'}
        <header class="group-heading">
          <h3 id="config-{scope}-{idPrefix}-commands">Commands</h3>
          <p>How commands are invoked and which words trigger them</p>
        </header>
      {:else}
        <h3 class="visually-hidden" id="config-{scope}-{idPrefix}-commands">Commands</h3>
      {/if}

      <div class="rows">
        <div
          class="row-group"
          class:overridden={Object.hasOwn(patch, 'command_prefix')}
          class:changed={keyChanged('command_prefix')}
        >
          <div class="row-line">
            <span class="row-label">
              <label for="config-{scope}-{idPrefix}-prefix">Prefix</label>
              <HelpTip
                id="config-{scope}-{idPrefix}-prefix-tooltip"
                label="About the command prefix"
                text="Characters required before a command when prefix invocation is used. Editing the {scope ===
                'target'
                  ? 'built-in default'
                  : 'inherited value'} creates an override"
                align="start"
                compact
              />
            </span>
            <span class="row-spacer"></span>
            <span class="changed-tag">Unsaved</span>
            <input
              id="config-{scope}-{idPrefix}-prefix"
              class="prefix-input mono"
              value={effectiveValue(draft, inherited, 'command_prefix')}
              disabled={editorDisabled}
              oninput={(event) => setPrefix(event.currentTarget.value)}
            />
            <SegmentedControl
              name="config-{scope}-{idPrefix}-prefix-mode"
              label="Command prefix source"
              options={MODE_OPTIONS}
              value={hasOverride('command_prefix') ? 'custom' : 'default'}
              compact
              onSelect={(selection) =>
                selection === 'custom' ? useCustom('command_prefix') : useDefault('command_prefix')}
              disabled={editorDisabled}
            />
          </div>
        </div>

        <div
          class="row-group"
          class:overridden={Object.hasOwn(patch, 'allowed_commands')}
          class:changed={keyChanged('allowed_commands')}
        >
          <div class="row-line">
            <span class="row-label">
              <span class="label-text">Allowed commands</span>
              <HelpTip
                id="config-{scope}-{idPrefix}-commands-tooltip"
                label="About allowed commands"
                text="The command words Smyklot accepts. At least one must remain enabled. Editing the selection creates an override"
                align="start"
                compact
              />
            </span>
            <span class="row-spacer"></span>
            <span class="changed-tag">Unsaved</span>
            <SegmentedControl
              name="config-{scope}-{idPrefix}-commands-mode"
              label="Allowed commands source"
              options={MODE_OPTIONS}
              value={hasOverride('allowed_commands') ? 'custom' : 'default'}
              compact
              onSelect={(selection) =>
                selection === 'custom'
                  ? useCustom('allowed_commands')
                  : useDefault('allowed_commands')}
              disabled={editorDisabled}
            />
          </div>
          <div class="row-body">
            <div class="cmd-flow">
              {#each COMMANDS as command (command)}
                {@const checked = commandIsAllowed(allowedList, command)}
                <label class="check-tile">
                  <input
                    type="checkbox"
                    {checked}
                    disabled={editorDisabled || (checked && allowedCount === 1)}
                    onchange={() => toggleCommand(command)}
                  />
                  <span class="check-box" aria-hidden="true">
                    <svg viewBox="0 0 12 12"><path d="M2.2 6.4 4.9 9 9.8 3.2" /></svg>
                  </span>
                  <code>{command}</code>
                </label>
              {/each}
            </div>
          </div>
        </div>

        <div
          class="row-group"
          class:overridden={Object.hasOwn(patch, 'command_aliases')}
          class:changed={keyChanged('command_aliases')}
        >
          <div class="row-line">
            <span class="row-label" id="config-{scope}-{idPrefix}-aliases-heading">
              <span class="label-text">Aliases</span>
              <HelpTip
                id="config-{scope}-{idPrefix}-aliases-tooltip"
                label="About command aliases"
                text="Extra command words mapped to canonical commands. Changing an alias creates an override"
                align="start"
                compact
              />
            </span>
            <span class="row-spacer"></span>
            <span class="changed-tag">Unsaved</span>
            {#if hasOverride('command_aliases') && !editorDisabled}
              <button
                class="reset-link"
                type="button"
                onclick={() => {
                  useDefault('command_aliases');
                  closeComposer();
                }}
              >
                Reset to default
              </button>
            {/if}
            <SegmentedControl
              name="config-{scope}-{idPrefix}-aliases-mode"
              label="Command aliases source"
              options={MODE_OPTIONS}
              value={hasOverride('command_aliases') ? 'custom' : 'default'}
              compact
              onSelect={(selection) =>
                selection === 'custom'
                  ? useCustom('command_aliases')
                  : useDefault('command_aliases')}
              disabled={editorDisabled}
            />
          </div>
          <div
            class="row-body alias-flow"
            role="group"
            aria-labelledby="config-{scope}-{idPrefix}-aliases-heading"
          >
            {#each aliasEntries as [name, command] (name)}
              <span class="word-chip" class:added={savedAliases[name] !== command}>
                <span class="chip-from">{name}</span>
                <span class="chip-arrow" aria-hidden="true">→</span>
                <span class="chip-to">{command}</span>
                <button
                  class="chip-x"
                  aria-label="Delete alias {name}"
                  title="Delete alias {name}"
                  disabled={editorDisabled}
                  onclick={() => removeAlias(name)}
                >
                  <Icon name="close" size={13} />
                </button>
              </span>
            {:else}
              <span class="alias-empty">No aliases yet</span>
            {/each}

            {#if composerOpen}
              <form
                class="composer"
                aria-label="Add command alias"
                onsubmit={(event) => {
                  event.preventDefault();
                  addAlias();
                }}
              >
                <label class="visually-hidden" for="config-{scope}-{idPrefix}-alias">Alias</label>
                <input
                  id="config-{scope}-{idPrefix}-alias"
                  class="mono"
                  placeholder="alias"
                  bind:value={aliasName}
                  disabled={editorDisabled}
                  onkeydown={(event) => {
                    if (event.key === 'Escape') closeComposer();
                  }}
                />
                <span class="chip-arrow" aria-hidden="true">→</span>
                <label class="visually-hidden" for="config-{scope}-{idPrefix}-alias-command">
                  Command
                </label>
                <select
                  id="config-{scope}-{idPrefix}-alias-command"
                  class="mono"
                  bind:value={aliasCommand}
                  disabled={editorDisabled}
                >
                  {#each COMMANDS as command (command)}
                    <option value={command}>{command}</option>
                  {/each}
                </select>
                <button
                  type="submit"
                  class="composer-ok"
                  disabled={editorDisabled || aliasName.trim() === ''}
                >
                  Add
                </button>
                <button
                  type="button"
                  class="composer-cancel"
                  aria-label="Cancel adding alias"
                  onclick={closeComposer}
                >
                  <Icon name="close" size={13} />
                </button>
              </form>
            {:else}
              <button
                class="add-chip"
                type="button"
                disabled={editorDisabled}
                onclick={openComposer}
              >
                <Icon name="plus" size={13} />
                <span>Add alias</span>
              </button>
            {/if}
          </div>
        </div>
      </div>
    </section>
  {/if}

  {#if dirty}
    <div class="save-bar" class:save-bar-inline={scope === 'repository'} role="status">
      <span class="save-dot" aria-hidden="true"></span>
      <span class="save-count">
        {changedKeys.length} unsaved {changedKeys.length === 1 ? 'change' : 'changes'}
      </span>
      <button class="bar-ghost" type="button" disabled={editorDisabled} onclick={discard}>
        Discard
      </button>
      <button class="btn btn-signal" disabled={editorDisabled} onclick={save}>
        <span class="btn-label">{saving ? 'Saving…' : 'Save'}</span>
      </button>
    </div>
  {/if}
</div>

<style>
  .config-editor {
    container-type: inline-size;
  }

  .editor-section + .editor-section {
    margin-top: 1.375rem;
  }

  .group-heading {
    margin: 0 0.125rem 0.625rem;
  }

  .group-heading h3 {
    color: var(--brand-action);
    font-size: var(--font-size-micro);
    font-weight: 700;
    letter-spacing: 0.1em;
    margin: 0;
    text-transform: uppercase;
  }

  .group-heading p {
    color: var(--dim);
    font-size: var(--font-size-meta);
    margin: 0.1875rem 0 0;
    max-width: 60ch;
  }

  .rows {
    border: 1px solid var(--rule);
    border-radius: var(--r-ctl);
  }

  .rows > :first-child {
    border-radius: calc(var(--r-ctl) - 1px) calc(var(--r-ctl) - 1px) 0 0;
  }

  .rows > :last-child {
    border-radius: 0 0 calc(var(--r-ctl) - 1px) calc(var(--r-ctl) - 1px);
  }

  .row,
  .row-line {
    align-items: center;
    display: flex;
    gap: var(--space-2);
    min-height: 3.25rem;
    padding: var(--space-2) 0.875rem;
  }

  .row,
  .row-group {
    background: var(--strip);
    transition: background-color 160ms ease-out;
  }

  .row + .row,
  .row-group + .row-group {
    border-top: 1px solid var(--rule);
  }

  .row-body {
    padding: 0 0.875rem 0.875rem;
  }

  .row-label {
    align-items: center;
    display: flex;
    font-size: 0.875rem;
    font-weight: 600;
    gap: 0.125rem;
  }

  .row-label label {
    cursor: pointer;
  }

  .row-spacer {
    flex: 1;
  }

  .overridden .row-label::before,
  .changed .row-label::before {
    border-radius: 50%;
    content: '';
    flex: none;
    height: 6px;
    margin-right: var(--space-2);
    width: 6px;
  }

  .overridden .row-label::before {
    background: var(--brand-action);
  }

  .changed .row-label::before {
    background: var(--pending);
  }

  .row.changed,
  .row-group.changed {
    background: var(--pending-tint);
  }

  .changed-tag {
    border: 1px solid color-mix(in srgb, var(--pending) 45%, transparent);
    border-radius: var(--r-chip);
    color: var(--pending);
    display: none;
    flex: none;
    font: 700 0.625rem / 1 var(--sans);
    letter-spacing: 0.06em;
    padding: 3px 8px;
    text-transform: uppercase;
  }

  .changed .changed-tag {
    display: inline-block;
  }

  .reset-link {
    background: none;
    border: 0;
    border-radius: 6px;
    color: var(--dim);
    cursor: pointer;
    font: 600 var(--font-size-compact) / 1 var(--sans);
    padding: 4px 8px;
    visibility: hidden;
  }

  .row:hover .reset-link,
  .row:focus-within .reset-link,
  .row-group:hover .reset-link,
  .row-group:focus-within .reset-link {
    visibility: visible;
  }

  .reset-link:hover {
    background: var(--well);
    color: var(--text);
  }

  .prefix-input {
    background: var(--strip-lift);
    border: 1px solid var(--border-strong);
    border-radius: var(--r-ctl);
    color: var(--text);
    flex: none;
    font-size: var(--font-size-control);
    height: var(--control-height-compact);
    margin-right: 0.25rem;
    text-align: center;
    width: 4.5rem;
  }

  .prefix-input:focus-visible {
    border-color: var(--brand-action);
    outline: 2px solid var(--brand);
  }

  .cmd-flow {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .check-tile {
    align-items: center;
    background: var(--strip-lift);
    border: 1px solid var(--rule);
    border-radius: var(--r-ctl);
    cursor: pointer;
    display: inline-flex;
    gap: 0.5625rem;
    min-height: 2.25rem;
    padding: 0 0.8125rem 0 0.625rem;
    transition:
      background-color 120ms ease-out,
      border-color 120ms ease-out;
  }

  .check-tile:hover:not(:has(input:disabled)) {
    border-color: var(--border-strong);
  }

  .check-tile:has(input:disabled) {
    cursor: default;
  }

  .check-tile input {
    height: 1px;
    opacity: 0;
    position: absolute;
    width: 1px;
  }

  .check-box {
    background: var(--strip);
    border: 1.5px solid var(--border-strong);
    border-radius: 5px;
    flex: none;
    height: 1.125rem;
    position: relative;
    transition:
      background-color 130ms ease-out,
      border-color 130ms ease-out;
    width: 1.125rem;
  }

  .check-box svg {
    fill: none;
    height: 12px;
    inset: 0;
    margin: auto;
    position: absolute;
    stroke: var(--on-admin);
    stroke-dasharray: 14;
    stroke-dashoffset: 14;
    stroke-linecap: round;
    stroke-linejoin: round;
    stroke-width: 2.4;
    transition: stroke-dashoffset 160ms var(--ease-standard) 40ms;
    width: 12px;
  }

  .check-tile input:checked + .check-box {
    background: var(--admin);
    border-color: var(--admin);
  }

  .check-tile input:checked + .check-box svg {
    stroke-dashoffset: 0;
  }

  .check-tile input:focus-visible + .check-box {
    outline: 2px solid var(--brand);
    outline-offset: 2px;
  }

  .check-tile input:disabled + .check-box {
    opacity: 0.7;
  }

  .check-tile code {
    background: transparent;
    color: var(--dim);
    font-size: var(--font-size-control);
    padding: 0;
    transition: color 120ms ease-out;
  }

  .check-tile input:checked ~ code {
    color: var(--text);
    font-weight: 500;
  }

  .alias-flow {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .word-chip {
    align-items: center;
    background: var(--strip-lift);
    border: 1px solid var(--rule);
    border-radius: var(--r-chip);
    display: inline-flex;
    font: 500 var(--font-size-control) / 1 var(--mono);
    gap: 0.4375rem;
    min-height: 2rem;
    padding: 0 0.375rem 0 0.875rem;
  }

  .word-chip.added {
    background: var(--brand-action-tint);
    border-color: var(--brand-action);
  }

  .chip-from {
    color: var(--text);
    font-weight: 500;
  }

  .chip-arrow {
    color: var(--dim);
    font-size: var(--font-size-micro);
  }

  .chip-to {
    color: var(--brand-action-text);
  }

  .chip-x {
    align-items: center;
    background: none;
    border: 0;
    border-radius: 50%;
    color: var(--dim);
    cursor: pointer;
    display: inline-flex;
    height: 1.25rem;
    justify-content: center;
    padding: 0;
    width: 1.25rem;
  }

  .chip-x:hover:not(:disabled) {
    background: var(--stop-tint);
    color: var(--stop);
  }

  .alias-empty {
    color: var(--dim);
    font-size: var(--font-size-meta);
  }

  .add-chip {
    align-items: center;
    background: none;
    border: 1.5px dashed var(--border-strong);
    border-radius: var(--r-chip);
    color: var(--text-secondary);
    cursor: pointer;
    display: inline-flex;
    font: 600 var(--font-size-compact) / 1 var(--sans);
    gap: 0.375rem;
    min-height: 2rem;
    padding: 0 0.875rem;
  }

  .add-chip:hover:not(:disabled) {
    background: var(--brand-action-tint);
    border-color: var(--brand-action);
    color: var(--brand-action-text);
  }

  .composer {
    align-items: center;
    background: var(--strip-lift);
    border: 1px solid var(--brand-action);
    border-radius: var(--r-chip);
    display: inline-flex;
    gap: 0.375rem;
    min-height: 2rem;
    padding: 2px 4px 2px 2px;
  }

  .composer:focus-within {
    box-shadow: 0 0 0 1px var(--brand-action);
  }

  .composer input,
  .composer select {
    background: var(--strip);
    border: 0;
    border-radius: var(--r-chip);
    color: var(--text);
    font-size: var(--font-size-control);
    height: 1.625rem;
  }

  .composer input {
    padding: 0 0.625rem;
    width: 6rem;
  }

  .composer input:focus {
    outline: none;
  }

  .composer select {
    padding: 0 0.375rem;
  }

  .composer-ok {
    background: var(--admin);
    border: 0;
    border-radius: var(--r-chip);
    color: var(--on-admin);
    cursor: pointer;
    font: 600 var(--font-size-compact) / 1 var(--sans);
    height: 1.625rem;
    padding: 0 0.75rem;
  }

  .composer-cancel {
    align-items: center;
    background: none;
    border: 0;
    border-radius: 50%;
    color: var(--dim);
    cursor: pointer;
    display: inline-flex;
    height: 1.5rem;
    justify-content: center;
    padding: 0;
    width: 1.5rem;
  }

  .composer-cancel:hover {
    background: var(--well);
    color: var(--text);
  }

  .save-bar {
    align-items: center;
    animation: save-bar-rise 240ms var(--ease-standard);
    background: var(--text-primary);
    border-radius: 12px;
    bottom: 1.25rem;
    box-shadow: 0 12px 32px rgb(0 0 0 / 30%);
    color: var(--canvas);
    display: flex;
    font: 600 var(--font-size-control) / 1 var(--sans);
    gap: 0.875rem;
    left: 50%;
    padding: 0.625rem 0.75rem 0.625rem 1rem;
    position: fixed;
    transform: translateX(-50%);
    z-index: var(--layer-sticky);
  }

  /* Trim text boxes to glyph bounds so flex centering is visually exact.
     Labels inside flex containers need their own span: trimming must happen
     on the flex item that holds the text, not on the container. */
  .save-count,
  .bar-ghost,
  .save-bar .btn-label,
  .row-label .label-text,
  .row-label label,
  .changed-tag,
  .reset-link,
  .check-tile code,
  .chip-from,
  .chip-arrow,
  .chip-to,
  .alias-empty,
  .add-chip span,
  .composer-ok {
    text-box: trim-both cap alphabetic;
  }

  .save-bar-inline {
    animation: none;
    bottom: auto;
    justify-content: flex-end;
    left: auto;
    margin-top: 0.75rem;
    position: static;
    transform: none;
  }

  @keyframes save-bar-rise {
    from {
      opacity: 0;
      transform: translate(-50%, 1rem);
    }

    to {
      opacity: 1;
      transform: translate(-50%, 0);
    }
  }

  .save-dot {
    animation: save-dot-pulse 1.6s ease-in-out infinite;
    background: var(--pending-inverse);
    border-radius: 50%;
    flex: none;
    height: 8px;
    width: 8px;
  }

  @keyframes save-dot-pulse {
    0%,
    100% {
      opacity: 1;
    }

    50% {
      opacity: 0.35;
    }
  }

  .bar-ghost {
    background: none;
    border: 0;
    border-radius: var(--r-ctl);
    color: inherit;
    cursor: pointer;
    font: 600 var(--font-size-control) / 1 var(--sans);
    opacity: 0.75;
    padding: 0.5rem 0.625rem;
  }

  .save-bar-inline .save-dot {
    background: var(--pending);
  }

  .save-bar-inline .bar-ghost,
  .save-bar-inline .save-count {
    color: var(--text-secondary);
  }

  .save-bar-inline {
    background: transparent;
    box-shadow: none;
    color: var(--text-secondary);
  }

  .bar-ghost:hover:not(:disabled) {
    background: rgb(255 255 255 / 12%);
    opacity: 1;
  }

  .save-bar-inline .bar-ghost:hover:not(:disabled) {
    background: var(--well);
  }

  @media (prefers-reduced-motion: reduce) {
    .save-bar {
      animation: none;
    }

    .save-dot {
      animation: none;
    }

    .check-box svg {
      transition: none;
    }
  }
</style>

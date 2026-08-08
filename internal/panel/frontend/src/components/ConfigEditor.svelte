<script lang="ts">
  import { untrack } from 'svelte';

  import {
    BOOLEAN_FIELDS,
    clonePatch,
    commandIsAllowed,
    effectiveValue,
    patchesEqual,
    reconcilePatchDraft,
    setExplicitPatchValue,
    toggleAllowedCommand,
    updatePatchValue,
  } from '../lib/config';
  import { COMMANDS } from '../lib/types';
  import type { ConfigKey, ConfigPatch, ConfigValues } from '../lib/types';
  import HelpTip from './HelpTip.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  const BOOLEAN_OPTIONS = [
    { value: 'default', label: 'Default' },
    { value: 'on', label: 'On', tone: 'on' },
    { value: 'off', label: 'Off', tone: 'off' },
  ] as const;

  const MODE_OPTIONS = [
    { value: 'default', label: 'Default' },
    { value: 'custom', label: 'Custom' },
  ] as const;

  const {
    patch,
    inherited,
    scope,
    idPrefix,
    disabled = false,
    onSave,
  }: {
    patch: ConfigPatch;
    inherited: ConfigValues;
    scope: 'target' | 'repository';
    idPrefix: string;
    disabled?: boolean;
    onSave: (next: ConfigPatch) => Promise<void>;
  } = $props();

  const initialPatch = clonePatch(untrack(() => patch));
  let draft = $state<ConfigPatch>(initialPatch);
  let receivedPatch = $state<ConfigPatch>(clonePatch(initialPatch));
  let saving = $state(false);
  let aliasName = $state('');
  let aliasCommand = $state('approve');

  const dirty = $derived(!patchesEqual(draft, patch));
  const editorDisabled = $derived(disabled || saving);
  const aliasEntries = $derived(
    Object.entries(effectiveValue(draft, inherited, 'command_aliases')),
  );

  $effect(() => {
    const incoming = clonePatch(patch);
    const nextDraft = reconcilePatchDraft(draft, receivedPatch, incoming);
    if (nextDraft === draft) return;
    receivedPatch = incoming;
    draft = nextDraft;
  });

  function hasOverride(key: ConfigKey): boolean {
    return Object.hasOwn(draft, key);
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

  function setBoolean(key: (typeof BOOLEAN_FIELDS)[number]['key'], value: boolean): void {
    draft = setExplicitPatchValue(draft, key, value);
  }

  function selectBoolean(key: (typeof BOOLEAN_FIELDS)[number]['key'], selection: string): void {
    if (selection === 'default') {
      useDefault(key);
      return;
    }
    setBoolean(key, selection === 'on');
  }

  function setPrefix(value: string): void {
    draft = updatePatchValue(draft, inherited, 'command_prefix', value);
  }

  function toggleCommand(command: string): void {
    const current = effectiveValue(draft, inherited, 'allowed_commands');
    const next = toggleAllowedCommand(current, command, COMMANDS);
    draft = updatePatchValue(draft, inherited, 'allowed_commands', next);
  }

  function addAlias(): void {
    const name = aliasName.trim();
    if (name === '') return;
    const current = effectiveValue(draft, inherited, 'command_aliases');
    draft = updatePatchValue(draft, inherited, 'command_aliases', {
      ...current,
      [name]: aliasCommand,
    });
    aliasName = '';
  }

  function removeAlias(name: string): void {
    const next = { ...effectiveValue(draft, inherited, 'command_aliases') };
    delete next[name];
    draft = updatePatchValue(draft, inherited, 'command_aliases', next);
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

  function cloneValue<T>(value: T): T {
    return JSON.parse(JSON.stringify(value)) as T;
  }
</script>

<div class="config-editor">
  <section class="editor-section" aria-labelledby="config-{scope}-behavior">
    <header class="group-heading">
      <h4 id="config-{scope}-behavior">Behavior</h4>
      <p>Comments, mentions, reactions, and approval safeguards</p>
    </header>

    <div class="boolean-grid">
      {#each BOOLEAN_FIELDS as field (field.key)}
        {@const overridden = hasOverride(field.key)}
        {@const value = effectiveValue(draft, inherited, field.key)}
        <div class="boolean-row">
          <div class="config-copy">
            <span class="field-label">{field.label}</span>
            <span class="visually-hidden" id="config-{scope}-{idPrefix}-{field.key}-help">
              {field.help}
            </span>
          </div>
          <HelpTip
            id="config-{scope}-{idPrefix}-{field.key}-tooltip"
            label="About {field.label.toLowerCase()}"
            text={field.help}
            compact
          />
          <SegmentedControl
            name="config-{scope}-{idPrefix}-{field.key}"
            label={field.label}
            descriptionId="config-{scope}-{idPrefix}-{field.key}-help"
            options={BOOLEAN_OPTIONS}
            value={overridden ? (value ? 'on' : 'off') : 'default'}
            onSelect={(selection) => selectBoolean(field.key, selection)}
            disabled={editorDisabled}
          />
        </div>
      {/each}
    </div>
  </section>

  <section class="editor-section" aria-labelledby="config-{scope}-commands">
    <header class="group-heading">
      <h4 id="config-{scope}-commands">Commands</h4>
      <p>Invocation syntax, available actions, and aliases</p>
    </header>

    <div class="command-fields">
      <div class="config-row">
        <div class="config-copy">
          <label for="config-{scope}-{idPrefix}-prefix">Prefix</label>
        </div>
        <div class="config-value">
          <HelpTip
            id="config-{scope}-{idPrefix}-prefix-tooltip"
            label="About command prefix"
            text="Characters required before a command when prefix invocation is used. Editing the inherited value creates a custom setting"
            compact
          />
          <input
            id="config-{scope}-{idPrefix}-prefix"
            class="text-input mono short-input"
            class:inherited-value={!hasOverride('command_prefix')}
            value={effectiveValue(draft, inherited, 'command_prefix')}
            disabled={editorDisabled}
            oninput={(event) => setPrefix(event.currentTarget.value)}
          />
          <SegmentedControl
            name="config-{scope}-{idPrefix}-prefix-mode"
            label="Command prefix source"
            options={MODE_OPTIONS}
            value={hasOverride('command_prefix') ? 'custom' : 'default'}
            onSelect={(selection) =>
              selection === 'custom' ? useCustom('command_prefix') : useDefault('command_prefix')}
            disabled={editorDisabled}
          />
        </div>
      </div>

      <div class="config-row config-stack">
        <div class="config-copy">
          <span class="field-label">Allowed</span>
        </div>
        <HelpTip
          id="config-{scope}-{idPrefix}-commands-tooltip"
          label="About allowed commands"
          text="Choose which canonical commands Smyklot accepts. All commands are enabled by default, and at least one must remain enabled. Editing the inherited selection creates a custom setting"
          compact
        />
        <SegmentedControl
          name="config-{scope}-{idPrefix}-commands-mode"
          label="Allowed commands source"
          options={MODE_OPTIONS}
          value={hasOverride('allowed_commands') ? 'custom' : 'default'}
          align="end"
          onSelect={(selection) =>
            selection === 'custom' ? useCustom('allowed_commands') : useDefault('allowed_commands')}
          disabled={editorDisabled}
        />
        <div class="command-grid" class:inherited-value={!hasOverride('allowed_commands')}>
          {#each COMMANDS as command (command)}
            <label class="check-option">
              <input
                type="checkbox"
                checked={commandIsAllowed(
                  effectiveValue(draft, inherited, 'allowed_commands'),
                  command,
                )}
                disabled={editorDisabled}
                onchange={() => toggleCommand(command)}
              />
              <span class="check-box" aria-hidden="true"></span>
              <code>{command}</code>
            </label>
          {/each}
        </div>
      </div>

      <div class="config-row config-stack">
        <div class="config-copy">
          <span class="field-label" id="config-{scope}-{idPrefix}-aliases-heading">Aliases</span>
        </div>
        <HelpTip
          id="config-{scope}-{idPrefix}-aliases-tooltip"
          label="About command aliases"
          text="Map additional command words to canonical Smyklot commands. Adding, changing, or deleting an inherited alias creates a custom setting"
          compact
        />
        <SegmentedControl
          name="config-{scope}-{idPrefix}-aliases-mode"
          label="Command aliases source"
          options={MODE_OPTIONS}
          value={hasOverride('command_aliases') ? 'custom' : 'default'}
          align="end"
          onSelect={(selection) =>
            selection === 'custom' ? useCustom('command_aliases') : useDefault('command_aliases')}
          disabled={editorDisabled}
        />

        <div
          class="alias-table"
          class:inherited-value={!hasOverride('command_aliases')}
          role="group"
          aria-labelledby="config-{scope}-{idPrefix}-aliases-heading"
        >
          {#each aliasEntries as [name, command] (name)}
            <div class="alias-row">
              <code>{name}</code>
              <span class="alias-arrow" aria-hidden="true">→</span>
              <code>{command}</code>
              <button
                class="alias-delete"
                aria-label="Delete alias {name}"
                title="Delete alias {name}"
                disabled={editorDisabled}
                onclick={() => removeAlias(name)}
              >
                <svg viewBox="0 0 20 20" aria-hidden="true">
                  <path d="M4.5 5.5h11M8 5.5V3.75h4v1.75m2 0-.5 10.75h-7L6 5.5"></path>
                  <path d="M8.5 8v5.75M11.5 8v5.75"></path>
                </svg>
              </button>
            </div>
          {:else}
            <p class="alias-empty">No aliases configured</p>
          {/each}

          <form
            class="alias-create"
            aria-label="Add command alias"
            onsubmit={(event) => {
              event.preventDefault();
              addAlias();
            }}
          >
            <label class="visually-hidden" for="config-{scope}-{idPrefix}-alias">Alias</label>
            <input
              id="config-{scope}-{idPrefix}-alias"
              class="text-input mono"
              placeholder="Alias"
              bind:value={aliasName}
              disabled={editorDisabled}
            />
            <span class="alias-arrow" aria-hidden="true">→</span>
            <label class="visually-hidden" for="config-{scope}-{idPrefix}-alias-command"
              >Command</label
            >
            <select
              id="config-{scope}-{idPrefix}-alias-command"
              class="select-input mono"
              bind:value={aliasCommand}
              disabled={editorDisabled}
            >
              {#each COMMANDS as command (command)}
                <option value={command}>{command}</option>
              {/each}
            </select>
            <button
              type="submit"
              class="btn btn-signal alias-add"
              disabled={editorDisabled || aliasName.trim() === ''}
            >
              <svg viewBox="0 0 16 16" aria-hidden="true">
                <path d="M8 3v10M3 8h10"></path>
              </svg>
              <span>Add</span>
            </button>
          </form>
        </div>
      </div>
    </div>
  </section>

  <div class="config-actions">
    <button class="btn btn-signal" disabled={editorDisabled || !dirty} onclick={save}>
      {saving ? 'Saving…' : 'Save'}
    </button>
  </div>
</div>

<style>
  .config-editor {
    container-type: inline-size;
  }

  .editor-section + .editor-section {
    margin-top: 0.875rem;
  }

  .group-heading {
    align-items: baseline;
    display: flex;
    flex-wrap: wrap;
    gap: 0.15rem 0.65rem;
    margin-bottom: 0.4rem;
    padding-inline: 0.75rem;
  }

  .group-heading h4 {
    font-size: 0.8125rem;
    margin: 0;
  }

  .group-heading p {
    color: var(--dim);
    font-size: 0.75rem;
    margin: 0;
  }

  .boolean-grid,
  .command-fields {
    background: var(--rule);
    border-radius: var(--r-well);
    display: grid;
    gap: 1px;
    overflow: hidden;
  }

  .boolean-grid {
    grid-template-columns: 1fr;
  }

  .boolean-row,
  .config-row {
    align-items: center;
    background: var(--strip-lift);
    display: grid;
    gap: 0.4rem 0.75rem;
    grid-template-columns: minmax(0, 1fr) auto;
    min-height: 3.5rem;
    padding: 0.45rem 0.75rem;
  }

  .boolean-row {
    column-gap: 2px;
    grid-template-columns: minmax(0, 1fr) auto auto;
    min-height: 3.5rem;
  }

  .config-copy label,
  .field-label {
    display: block;
    font-size: 0.8125rem;
    font-weight: 600;
    line-height: 1.2;
  }

  .inherited-value {
    opacity: 0.5;
    transition: opacity 120ms ease-out;
  }

  .config-value {
    align-items: center;
    display: flex;
    gap: 2px;
  }

  .text-input,
  .select-input {
    background-color: var(--input-surface);
  }

  .config-stack {
    column-gap: 2px;
    grid-template-columns: minmax(0, 1fr) auto auto;
  }

  .config-stack .command-grid,
  .config-stack .alias-table {
    grid-column: 1 / -1;
  }

  .command-grid {
    display: grid;
    gap: 0.35rem;
    grid-template-columns: repeat(auto-fit, minmax(7.25rem, 1fr));
  }

  .check-option {
    align-items: center;
    background: var(--input-surface);
    border-radius: calc(var(--r-ctl) - 2px);
    cursor: pointer;
    display: flex;
    gap: 0.45rem;
    height: var(--control-height);
    padding: 0 0.5rem;
  }

  .check-option input {
    height: 1px;
    opacity: 0;
    position: absolute;
    width: 1px;
  }

  .check-box {
    background: var(--strip);
    border: 1px solid var(--dim);
    border-radius: 4px;
    flex: none;
    height: 1rem;
    position: relative;
    width: 1rem;
  }

  .check-box::after {
    border-bottom: 2px solid var(--on-signal);
    border-right: 2px solid var(--on-signal);
    content: '';
    height: 0.48rem;
    left: 0.32rem;
    opacity: 0;
    position: absolute;
    top: 0.14rem;
    transform: rotate(45deg);
    width: 0.25rem;
  }

  .check-option input:checked + .check-box {
    background: var(--signal);
    border-color: var(--signal);
  }

  .check-option input:checked + .check-box::after {
    opacity: 1;
  }

  .check-option input:focus-visible + .check-box {
    outline: 2px solid var(--brand);
    outline-offset: 2px;
  }

  .check-option input:disabled + .check-box,
  .check-option input:disabled ~ code {
    opacity: 0.45;
  }

  .check-option code {
    background: transparent;
    padding: 0;
  }

  .alias-table {
    border: 1px solid var(--rule);
    border-radius: var(--r-ctl);
    display: grid;
    grid-template-columns: minmax(0, 1fr) 1.5rem minmax(8rem, 1fr) auto;
    overflow: hidden;
  }

  .alias-row,
  .alias-create {
    align-items: center;
    display: grid;
    gap: 0.5rem;
    grid-column: 1 / -1;
    grid-template-columns: subgrid;
  }

  .alias-row {
    min-height: calc(var(--control-height) + 0.5rem);
    padding: 0.25rem 0.625rem;
  }

  .alias-row code {
    justify-self: start;
  }

  .alias-row code:first-child {
    justify-self: end;
  }

  .alias-arrow {
    align-items: center;
    color: var(--dim);
    display: flex;
    flex: none;
    font-size: 0.75rem;
    justify-content: center;
    line-height: 1;
    place-self: stretch;
  }

  .alias-delete {
    align-items: center;
    background: transparent;
    border: 1px solid transparent;
    border-radius: var(--r-ctl);
    color: var(--dim);
    display: inline-flex;
    height: var(--control-height);
    justify-content: center;
    padding: 0;
    transition:
      background-color 120ms ease-out,
      border-color 120ms ease-out,
      color 120ms ease-out,
      transform 90ms ease-out;
    width: var(--control-height);
    justify-self: end;
  }

  .alias-delete:hover:not(:disabled) {
    background: var(--stop-tint);
    border-color: color-mix(in srgb, var(--stop) 45%, transparent);
    color: var(--stop);
  }

  .alias-delete:active:not(:disabled) {
    background: var(--stop-tint);
    border-color: var(--stop);
    color: var(--stop);
    transform: translateY(1px);
  }

  .alias-delete:disabled {
    cursor: default;
    opacity: 0.5;
  }

  .alias-delete svg {
    fill: none;
    height: 1rem;
    stroke: currentColor;
    stroke-linecap: round;
    stroke-linejoin: round;
    stroke-width: 1.5;
    width: 1rem;
  }

  .alias-empty {
    color: var(--dim);
    font-size: 0.75rem;
    grid-column: 1 / -1;
    margin: 0;
    padding: 0.65rem;
  }

  .alias-create {
    padding: 0.5rem 0.625rem;
  }

  .alias-row + .alias-row,
  .alias-row + .alias-create,
  .alias-empty + .alias-create {
    border-top: 1px solid var(--rule);
  }

  .alias-create .text-input,
  .alias-create .select-input {
    font-size: 0.75rem;
    height: 1.875rem;
    min-width: 0;
    width: 100%;
  }

  .alias-create .text-input {
    padding-inline: 0.5rem;
  }

  .alias-create .select-input {
    background-position:
      calc(100% - 0.8rem) 50%,
      calc(100% - 0.55rem) 50%;
    padding-left: 0.5rem;
    padding-right: 1.75rem;
  }

  .alias-add {
    font-size: 0.75rem;
    height: 1.875rem;
    min-width: 4.5rem;
    padding-inline: 0.625rem;
  }

  .alias-add svg {
    fill: none;
    height: 0.75rem;
    stroke: currentColor;
    stroke-linecap: round;
    stroke-width: 1.75;
    width: 0.75rem;
  }

  .short-input {
    margin-right: 0.4rem;
    width: 5rem;
  }

  .config-actions {
    display: flex;
    justify-content: flex-end;
    padding-top: 0.75rem;
  }

  @container (min-width: 36rem) {
    .boolean-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }

    .boolean-grid:has(.boolean-row:last-child:nth-child(odd))::after {
      background: var(--strip-lift);
      content: '';
      min-height: 3.5rem;
    }
  }

  @container (min-width: 56rem) {
    .boolean-grid {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }

    .boolean-grid:has(.boolean-row:last-child:nth-child(odd))::after {
      content: none;
    }
  }

  @media (max-width: 38rem) {
    .boolean-grid,
    .command-fields > .config-row:not(.config-stack) {
      grid-template-columns: 1fr;
    }

    .config-value {
      justify-self: start;
    }

    .alias-table {
      grid-template-columns: minmax(0, 1fr) 1.5rem minmax(0, 1fr) auto;
    }

    .alias-add {
      grid-column: 1 / -1;
      width: 100%;
    }
  }
</style>

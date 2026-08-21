<script lang="ts">
  import { untrack } from 'svelte';

  import {
    BOOLEAN_FIELDS,
    clonePatch,
    commandIsAllowed,
    effectiveValue,
    fieldEnabled,
    fieldRawValue,
    reconcilePatchDraft,
    setExplicitPatchValue,
    toggleAllowedCommand,
    updatePatchValue,
  } from '../config';
  import type { BooleanField } from '../config';
  import { COMMANDS } from '../types';
  import type { ConfigKey, ConfigPatch, ConfigValues } from '../types';
  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import Popover from './Popover.svelte';
  import Switch from './Switch.svelte';

  /* The linked-value rows name their inheritance source per scope. */
  const SOURCE_BY_SCOPE = {
    target: 'the application defaults',
    repository: 'workspace defaults',
    runtime: 'the deployment configuration',
  } as const;

  const {
    patch,
    inherited,
    scope,
    idPrefix,
    disabled = false,
    section = 'all',
    only,
    onSave,
  }: {
    patch: ConfigPatch;
    inherited: ConfigValues;
    scope: 'target' | 'repository' | 'runtime';
    idPrefix: string;
    disabled?: boolean;
    section?: 'all' | 'behavior' | 'commands';
    /** Render only these behavior rows. Used by the repository-file pane, which
     *  shows the overrides in effect rather than the whole settings list. */
    only?: readonly ConfigKey[];
    /** Rejects when the save was refused - the receipt only shows on success. */
    onSave: (next: ConfigPatch) => Promise<void>;
  } = $props();

  const source = $derived(SOURCE_BY_SCOPE[scope]);
  const shownFields = $derived(
    only === undefined
      ? BOOLEAN_FIELDS
      : BOOLEAN_FIELDS.filter((field) => only.includes(field.key)),
  );

  const initialPatch = clonePatch(untrack(() => patch));
  let draft = $state<ConfigPatch>(initialPatch);
  let receivedPatch = $state<ConfigPatch>(clonePatch(initialPatch));
  let saving = $state(false);
  let picking = $state(false);
  let aliasOpen = $state(false);
  let aliasName = $state('');

  const editorDisabled = $derived(disabled || saving);
  const overriddenFields = $derived(shownFields.filter((field) => Object.hasOwn(draft, field.key)));
  const restFields = $derived(shownFields.filter((field) => !Object.hasOwn(draft, field.key)));
  const aliasEntries = $derived(
    Object.entries(effectiveValue(draft, inherited, 'command_aliases')),
  );
  const allowedList = $derived(effectiveValue(draft, inherited, 'allowed_commands'));
  const allowedCount = $derived(allowedList.length === 0 ? COMMANDS.length : allowedList.length);
  const commandKeys: readonly ConfigKey[] = [
    'command_prefix',
    'allowed_commands',
    'command_aliases',
  ];
  const commandsOverridden = $derived(
    commandKeys.filter((key) => Object.hasOwn(draft, key)).length,
  );
  const cleanAlias = $derived(aliasName.trim());
  const aliasTaken = $derived(
    cleanAlias !== '' &&
      Object.hasOwn(effectiveValue(draft, inherited, 'command_aliases'), cleanAlias),
  );

  $effect(() => {
    const incoming = clonePatch(patch);
    const nextDraft = reconcilePatchDraft(draft, receivedPatch, incoming);
    if (nextDraft === draft) return;
    receivedPatch = incoming;
    draft = nextDraft;
  });

  /* ---------- The saved receipts, one per card ---------- */

  let behaviorSavedOn = $state(false);
  let commandsSavedOn = $state(false);
  let behaviorTimer: ReturnType<typeof setTimeout> | undefined;
  let commandsTimer: ReturnType<typeof setTimeout> | undefined;

  function whisper(card: 'behavior' | 'commands'): void {
    if (card === 'behavior') {
      behaviorSavedOn = true;
      clearTimeout(behaviorTimer);
      behaviorTimer = setTimeout(() => (behaviorSavedOn = false), 1400);
    } else {
      commandsSavedOn = true;
      clearTimeout(commandsTimer);
      commandsTimer = setTimeout(() => (commandsSavedOn = false), 1400);
    }
  }

  async function push(card: 'behavior' | 'commands'): Promise<void> {
    if (saving) return;
    saving = true;
    try {
      await onSave(clonePatch(draft));
      whisper(card);
    } catch {
      /* The parent reports the refusal; the draft keeps what was asked so the
         next change carries it again. */
    } finally {
      saving = false;
    }
  }

  /* ---------- Behavior ---------- */

  function toggleBoolean(field: BooleanField, enabled: boolean): void {
    draft = setExplicitPatchValue(draft, field.key, fieldRawValue(field, enabled));
    void push('behavior');
  }

  function clearField(key: ConfigKey, card: 'behavior' | 'commands'): void {
    if (!Object.hasOwn(draft, key)) return;
    const next = { ...draft };
    delete next[key];
    draft = next;
    void push(card);
  }

  /* Overriding pins what inheritance resolves to today; the switch beside it
     is how a different value is chosen. */
  function manage(field: BooleanField): void {
    draft = setExplicitPatchValue(draft, field.key, cloneValue(inherited[field.key]));
    picking = false;
    void push('behavior');
  }

  /* ---------- Commands ---------- */

  let prefixTimer: ReturnType<typeof setTimeout> | undefined;
  const SAVE_REST_MS = 900;

  function typePrefix(value: string): void {
    draft = updatePatchValue(draft, inherited, 'command_prefix', value);
    clearTimeout(prefixTimer);
    prefixTimer = setTimeout(() => void push('commands'), SAVE_REST_MS);
  }

  function flushPrefix(): void {
    if (prefixTimer === undefined) return;
    clearTimeout(prefixTimer);
    prefixTimer = undefined;
    void push('commands');
  }

  function toggleCommand(command: string): void {
    const next = toggleAllowedCommand(allowedList, command, COMMANDS);
    draft = updatePatchValue(draft, inherited, 'allowed_commands', next);
    void push('commands');
  }

  function addAlias(): void {
    if (cleanAlias === '' || aliasTaken) return;
    const current = effectiveValue(draft, inherited, 'command_aliases');
    draft = updatePatchValue(draft, inherited, 'command_aliases', {
      ...current,
      [cleanAlias]: 'approve',
    });
    aliasName = '';
    aliasOpen = false;
    void push('commands');
  }

  function retargetAlias(name: string, command: string): void {
    const current = effectiveValue(draft, inherited, 'command_aliases');
    draft = updatePatchValue(draft, inherited, 'command_aliases', {
      ...current,
      [name]: command,
    });
    void push('commands');
  }

  function removeAlias(name: string): void {
    const next = { ...effectiveValue(draft, inherited, 'command_aliases') };
    delete next[name];
    draft = updatePatchValue(draft, inherited, 'command_aliases', next);
    void push('commands');
  }

  function cloneValue<T>(value: T): T {
    return JSON.parse(JSON.stringify(value)) as T;
  }
</script>

{#snippet clearButton(key: ConfigKey, card: 'behavior' | 'commands', what: string)}
  <button
    class="setting-clear"
    title="Stop overriding - follow {source}"
    aria-label="Stop overriding {what}"
    disabled={editorDisabled}
    onclick={() => clearField(key, card)}
  >
    <Icon name="close" size={10} />
  </button>
{/snippet}

<div class="config-editor">
  {#if section === 'all' || section === 'behavior'}
    {#if only !== undefined}
      <!-- The repository-file pane's list: just the rows in effect, no card of
           its own - the pane already stands inside one. -->
      <div class="policy-rows">
        {#each overriddenFields as field (field.key)}
          {@const on = fieldEnabled(field, effectiveValue(draft, inherited, field.key))}
          <div class="policy-row">
            <span class="setting-say">
              <span class="setting-name">{field.label}</span>
              <span class="setting-why">{field.help}</span>
            </span>
            <span class="policy-value">
              <span class="value-word" class:is-on={on}>{on ? 'On' : 'Off'}</span>
              <Switch
                checked={on}
                label={field.label}
                disabled={editorDisabled}
                onToggle={(next) => toggleBoolean(field, next)}
              />
            </span>
            {@render clearButton(field.key, 'behavior', field.label)}
          </div>
        {/each}
      </div>
    {:else}
      <section class="card group-card" aria-labelledby="config-{scope}-{idPrefix}-behavior">
        <div class="group-head">
          <h3 class="group-name" id="config-{scope}-{idPrefix}-behavior">Behavior</h3>
          <span class="save-whisper" class:is-on={behaviorSavedOn} role="status"
            ><Icon name="check" size={12} /><span class="t">Saved</span></span
          >
          <span class="group-tally"
            >{overriddenFields.length} of {shownFields.length} overridden</span
          >
        </div>
        <p class="group-note">How Smyklot replies and which safeguards apply</p>
        {#if overriddenFields.length > 0}
          <div class="policy-rows">
            {#each overriddenFields as field (field.key)}
              {@const on = fieldEnabled(field, effectiveValue(draft, inherited, field.key))}
              <div class="policy-row">
                <span class="setting-say">
                  <span class="setting-name">{field.label}</span>
                  <span class="setting-why">{field.help}</span>
                </span>
                <span class="policy-value">
                  <span class="value-word" class:is-on={on}>{on ? 'On' : 'Off'}</span>
                  <Switch
                    checked={on}
                    label={field.label}
                    disabled={editorDisabled}
                    onToggle={(next) => toggleBoolean(field, next)}
                  />
                </span>
                {@render clearButton(field.key, 'behavior', field.label)}
              </div>
            {/each}
          </div>
        {/if}
        {#if restFields.length > 0}
          {@const names = restFields.map((field) => field.label)}
          <div class="group-rest" class:is-open={picking}>
            {#if picking}
              <span class="rest-say"
                ><span class="rest-count">{restFields.length} follow {source}</span> - pick one to override:</span
              >
              <span class="rest-picks">
                {#each restFields as field (field.key)}
                  <button class="add-chip" disabled={editorDisabled} onclick={() => manage(field)}>
                    <Icon name="plus" size={12} />
                    <span class="t">{field.label}</span>
                  </button>
                {/each}
                <Button tone="quiet" onclick={() => (picking = false)}>Cancel</Button>
              </span>
            {:else}
              <span class="rest-say"
                ><span class="rest-count">{restFields.length} follow {source}</span> - {names.join(
                  ', ',
                )}</span
              >
              <Button tone="quiet" disabled={editorDisabled} onclick={() => (picking = true)}>
                {#snippet icon()}<Icon name="plus" size={13} />{/snippet}
                Override one
              </Button>
            {/if}
          </div>
        {/if}
      </section>
    {/if}
  {/if}

  {#if only === undefined && (section === 'all' || section === 'commands')}
    <section class="card group-card" aria-labelledby="config-{scope}-{idPrefix}-commands">
      <div class="group-head">
        <h3 class="group-name" id="config-{scope}-{idPrefix}-commands">Commands</h3>
        <span class="save-whisper" class:is-on={commandsSavedOn} role="status"
          ><Icon name="check" size={12} /><span class="t">Saved</span></span
        >
        <span class="group-tally">{commandsOverridden} of 3 overridden</span>
      </div>
      <p class="group-note">How commands are invoked and which words trigger them</p>
      <div class="policy-rows">
        <div class="policy-row">
          <span class="setting-say">
            <label class="setting-name" for="config-{scope}-{idPrefix}-prefix">Prefix</label>
            <span class="setting-why"
              >Characters required before a command when prefix invocation is used. Editing the
              inherited value creates an override</span
            >
          </span>
          <span class="policy-value">
            <input
              id="config-{scope}-{idPrefix}-prefix"
              class="prefix-inline"
              value={effectiveValue(draft, inherited, 'command_prefix')}
              {disabled}
              oninput={(event) => typePrefix(event.currentTarget.value)}
              onblur={flushPrefix}
            />
          </span>
          {#if Object.hasOwn(draft, 'command_prefix')}
            {@render clearButton('command_prefix', 'commands', 'the command prefix')}
          {/if}
        </div>

        <div class="policy-row policy-block">
          <span class="setting-say">
            <span class="setting-name">Allowed commands</span>
            <span class="setting-why"
              >The command words Smyklot accepts. At least one must remain on</span
            >
          </span>
          <span class="policy-value">
            <span class="value-word is-on">{allowedCount} of {COMMANDS.length}</span>
          </span>
          {#if Object.hasOwn(draft, 'allowed_commands')}
            {@render clearButton('allowed_commands', 'commands', 'allowed commands')}
          {/if}
          <div class="chip-line" role="group" aria-label="Allowed commands">
            {#each COMMANDS as command (command)}
              {@const on = commandIsAllowed(allowedList, command)}
              <button
                class="cmd-chip"
                class:is-on={on}
                aria-pressed={on}
                disabled={editorDisabled || (on && allowedCount === 1)}
                onclick={() => toggleCommand(command)}
              >
                <Icon name={on ? 'check' : 'plus'} size={10} />
                <span class="t">{command}</span>
              </button>
            {/each}
          </div>
        </div>

        <div class="policy-row policy-block">
          <span class="setting-say">
            <span class="setting-name" id="config-{scope}-{idPrefix}-aliases">Aliases</span>
            <span class="setting-why">Extra command words mapped to the commands they invoke</span>
          </span>
          <span class="policy-value">
            <span class="value-word" class:is-on={aliasEntries.length > 0}
              >{aliasEntries.length === 0 ? 'none' : aliasEntries.length}</span
            >
          </span>
          {#if Object.hasOwn(draft, 'command_aliases')}
            {@render clearButton('command_aliases', 'commands', 'command aliases')}
          {/if}
          <div class="chip-line" role="group" aria-labelledby="config-{scope}-{idPrefix}-aliases">
            {#each aliasEntries as [name, command] (name)}
              <span class="alias-chip">
                <span class="t">{name}</span>
                <span class="alias-arrow" aria-hidden="true">→</span>
                <Popover
                  role="listbox"
                  label="Command {name} invokes"
                  align="start"
                  itemSelector=".menu-item"
                >
                  {#snippet trigger(attributes)}
                    <button
                      {...attributes}
                      class="alias-target"
                      type="button"
                      disabled={editorDisabled}
                      aria-label="Command {name} invokes"
                    >
                      <span class="t">{command}</span>
                    </button>
                  {/snippet}
                  <div class="menu-list">
                    {#each COMMANDS as candidate (candidate)}
                      <button
                        class="menu-item"
                        role="option"
                        aria-selected={candidate === command}
                        onclick={() => retargetAlias(name, candidate)}
                      >
                        <span class="menu-check">
                          {#if candidate === command}<Icon name="check" size={16} />{/if}
                        </span>
                        <span class="mi-label">{candidate}</span>
                      </button>
                    {/each}
                  </div>
                </Popover>
                <button
                  aria-label="Remove alias {name}"
                  disabled={editorDisabled}
                  onclick={() => removeAlias(name)}
                >
                  <Icon name="close" size={8} />
                </button>
              </span>
            {/each}
            <Popover role="dialog" label="Name the alias" align="start" bind:open={aliasOpen}>
              {#snippet trigger(attributes)}
                <button {...attributes} class="add-chip" disabled={editorDisabled}>
                  <Icon name="plus" size={12} />
                  <span class="t">Add an alias</span>
                </button>
              {/snippet}
              <div class="name-menu">
                <div class="menu-search">
                  <Icon name="search" size={12} />
                  <input
                    placeholder="ship"
                    aria-label="Name for the new alias"
                    spellcheck="false"
                    bind:value={aliasName}
                    onkeydown={(event) => {
                      if (event.key === 'Enter') addAlias();
                    }}
                  />
                </div>
                <div class="menu-hint">
                  {aliasTaken ? 'That word is taken' : 'Enter adds it as approve · retarget after'}
                </div>
              </div>
            </Popover>
          </div>
        </div>
      </div>
    </section>
  {/if}
</div>

<style>
  .config-editor {
    display: grid;
    gap: var(--space-4);
  }

  .card {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    padding: var(--space-5);
  }

  .group-head {
    align-items: end;
    display: flex;
    gap: var(--space-3);
    justify-content: space-between;
    margin-bottom: var(--space-2);
  }

  .group-name {
    font-size: var(--font-size-title);
    font-weight: 600;
    margin: 0;
    min-block-size: 12px;
    text-box: trim-both cap alphabetic;
  }

  .group-tally {
    color: var(--text-muted);
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    min-block-size: 8px;
    text-box: trim-both cap alphabetic;
  }

  .save-whisper {
    align-items: center;
    background: var(--success-tint);
    block-size: 20px;
    border-radius: var(--radius-chip);
    color: var(--success);
    display: inline-flex;
    font-size: var(--font-size-micro);
    font-weight: 600;
    gap: 4px;
    margin-inline-start: auto;
    opacity: 0;
    padding: 0 0.5rem;
    transition: opacity var(--duration-fast) var(--ease-standard);
  }

  .save-whisper.is-on {
    opacity: 1;
  }

  .save-whisper .t {
    text-box: trim-both cap alphabetic;
  }

  .group-note {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    margin: 0 0 var(--space-2);
    max-width: 60ch;
  }

  .policy-rows {
    display: grid;
  }

  .policy-row {
    align-items: center;
    display: grid;
    gap: var(--space-2) var(--space-4);
    grid-template-columns: 1fr auto auto;
    /* The halo hangs outside the text column, so row text keeps the card
       head's left edge. Whole numbers: 48 floor, 8px block padding. */
    margin-inline: calc(var(--space-2) * -1);
    min-block-size: 48px;
    /* The air around a drawn hairline is the card's own padding, on both
       sides; the edge rows shed it where no line follows, since the card
       edge already carries that inset. */
    padding: var(--space-5) var(--space-2);
    position: relative;
  }

  .policy-row:first-child {
    padding-block-start: var(--space-2);
  }

  .policy-row:last-child {
    padding-block-end: var(--space-2);
  }

  /* The remainder is a summary line, not a row - its boundary keeps the
     compact rhythm so the card does not end on a slab of air. */
  .policy-rows:has(+ .group-rest) > .policy-row:last-child {
    padding-block-end: var(--space-2);
  }

  /* A drawn hairline, not a border: a border on a radiused row curves at
     its tips and makes sibling rows measure one pixel apart. Every row owns
     the line under itself, so the unmanaged remainder needs none of its own
     and a card with no overridden rows shows no line at all. */
  .policy-row::after {
    background: var(--border-subtle);
    block-size: 1px;
    bottom: 0;
    content: '';
    inset-inline: var(--space-2);
    position: absolute;
  }

  .policy-row:last-child::after {
    content: none;
  }

  .policy-rows:has(+ .group-rest) > .policy-row:last-child::after {
    content: '';
  }

  .setting-say {
    display: grid;
    gap: var(--space-3);
  }

  .setting-name {
    font-size: var(--font-size-meta);
    font-weight: 600;
    min-block-size: 10px;
    text-box: trim-both cap alphabetic;
  }

  .setting-why {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    min-block-size: 9px;
    text-box: trim-both cap alphabetic;
  }

  .policy-value {
    align-items: center;
    display: flex;
    gap: var(--space-3);
    justify-self: end;
  }

  /* The value said in a word beside the control, so a scan reads the
     policy without decoding thumb positions. */
  .value-word {
    color: var(--text-muted);
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    min-inline-size: 1.9rem;
    text-align: end;
    text-box: trim-both cap alphabetic;
  }

  .value-word.is-on {
    color: var(--text-secondary);
    font-weight: 600;
  }

  .setting-clear {
    align-items: center;
    background: transparent;
    block-size: 26px;
    border: 0;
    border-radius: 50%;
    color: var(--text-muted);
    cursor: pointer;
    display: inline-flex;
    inline-size: 26px;
    justify-content: center;
    padding: 0;
  }

  .setting-clear:hover {
    background: var(--interactive-hover-layer);
    color: var(--text-primary);
  }

  .setting-clear:active {
    background: var(--interactive-pressed);
  }

  .policy-row .setting-clear {
    opacity: 0.45;
    transition: opacity var(--duration-fast) var(--ease-standard);
  }

  .policy-row:hover .setting-clear,
  .policy-row:focus-within .setting-clear {
    opacity: 1;
  }

  /* ---------- The command rows' own controls ---------- */

  .prefix-inline {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text-primary);
    font-family: var(--mono);
    font-size: var(--font-size-control);
    min-block-size: 28px;
    padding: 0;
    text-align: center;
    width: 4.5rem;
  }

  .prefix-inline:focus-visible {
    border-color: var(--brand-action);
    outline: 2px solid var(--brand);
  }

  /* A block row keeps the grid for its first line and lays its chips on a
     full-width second one. The extra breathing room lives INSIDE the row,
     above the chips - the block padding stays the shared 8px so the air
     around every hairline is the same on both sides. */
  .chip-line {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    grid-column: 1 / -1;
    margin-block: var(--space-1) 0;
  }

  .cmd-chip {
    align-items: center;
    background: var(--control-bg);
    border: 1px dashed var(--border-strong);
    border-radius: var(--radius-chip);
    color: var(--text-muted);
    cursor: pointer;
    display: inline-flex;
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    gap: 0.35rem;
    min-block-size: 30px;
    padding-block: 0;
    padding-inline: 0.7rem;
  }

  .cmd-chip:hover:not(:disabled) {
    background: var(--control-bg-hover);
    color: var(--text-primary);
  }

  .cmd-chip:active:not(:disabled) {
    background: var(--control-bg-pressed);
  }

  .cmd-chip.is-on {
    border-style: solid;
    color: var(--text-primary);
  }

  .cmd-chip:disabled {
    cursor: default;
    opacity: 0.6;
  }

  .cmd-chip .t {
    text-box: trim-both cap alphabetic;
  }

  .alias-chip {
    align-items: center;
    background: var(--surface-inset);
    block-size: 30px;
    border-radius: var(--radius-chip);
    color: var(--text-secondary);
    display: inline-flex;
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    gap: 0.35rem;
    line-height: 1;
    padding: 0 var(--space-2) 0 0.7rem;
  }

  .alias-chip .t {
    display: block;
    text-box: trim-both cap alphabetic;
  }

  .alias-arrow {
    color: var(--text-muted);
    /* Ink-true like its neighbours, so the chip's three parts share one
       centre instead of the arrow riding its line box's leading. */
    line-height: 1;
    text-box: trim-both cap alphabetic;
  }

  /* The command half is the pressable half: it opens the retarget menu. */
  .alias-target {
    background: none;
    border: 0;
    border-radius: var(--radius-chip);
    color: var(--text-primary);
    cursor: pointer;
    font: inherit;
    margin: -0.25rem;
    padding: 0.25rem;
  }

  .alias-target:hover:not(:disabled) {
    background: var(--interactive-hover-layer);
  }

  .alias-target[data-state='open'] {
    background: var(--interactive-pressed);
  }

  /* A 20px disc folded around an 8px glyph, the patch-chip x. */
  .alias-chip > button:last-child {
    align-items: center;
    background: none;
    border: 0;
    border-radius: 50%;
    color: inherit;
    cursor: pointer;
    display: inline-flex;
    margin: -0.375rem 0;
    opacity: 0.65;
    padding: 0.375rem;
  }

  .alias-chip > button:last-child:hover {
    background: var(--interactive-hover-layer);
    opacity: 1;
  }

  .alias-chip > button:last-child:active {
    background: var(--interactive-pressed);
  }

  .add-chip {
    align-items: center;
    background: var(--control-bg);
    border: 1px dashed var(--border-strong);
    border-radius: var(--radius-chip);
    color: var(--text-secondary);
    cursor: pointer;
    display: inline-flex;
    font-size: var(--font-size-compact);
    font-weight: 500;
    gap: 0.35rem;
    min-block-size: 30px;
    padding-block: 0;
    padding-inline: 0.7rem;
  }

  .add-chip:hover {
    background: var(--control-bg-hover);
    border-style: solid;
    color: var(--text-primary);
  }

  .add-chip:active {
    background: var(--control-bg-pressed);
  }

  .add-chip .t {
    text-box: trim-both cap alphabetic;
  }

  /* ---------- The menus ---------- */

  .menu-item {
    align-items: center;
    background: none;
    border: 0;
    border-radius: 6px;
    block-size: 32px;
    color: var(--text-primary);
    cursor: pointer;
    display: flex;
    font-size: var(--font-size-control);
    gap: var(--space-2);
    inline-size: 100%;
    padding-inline: var(--space-3);
    text-align: start;
  }

  .menu-item:hover {
    background: var(--interactive-hover-layer);
  }

  .menu-item:focus-visible {
    background: var(--interactive-hover-layer);
    outline: none;
  }

  .menu-item:active {
    background: var(--interactive-pressed);
  }

  .menu-check {
    display: inline-flex;
    flex: none;
    inline-size: 16px;
    justify-content: center;
  }

  .mi-label {
    font-family: var(--mono);
    min-inline-size: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* The menu's 4px mat - `.menu-search` bleeds to the edges with negative
     margins that assume exactly this pad. */
  .name-menu {
    display: grid;
    inline-size: 16rem;
    padding: var(--space-1);
  }

  .menu-search {
    align-items: center;
    block-size: 36px;
    box-shadow: 0 1px 0 var(--border-subtle);
    color: var(--text-muted);
    display: flex;
    gap: var(--space-2);
    margin: calc(var(--space-1) * -1) calc(var(--space-1) * -1) var(--space-1);
    padding: 0 var(--space-3);
  }

  .menu-search input {
    background: none;
    block-size: 100%;
    border: 0;
    color: var(--text-primary);
    flex: 1;
    font-size: var(--font-size-control);
    outline: none;
    padding: 0;
  }

  .menu-search input::placeholder {
    color: var(--text-muted);
  }

  .menu-hint {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    line-height: 16px;
    padding: var(--space-1) var(--space-3) var(--space-2);
  }

  /* ---------- The un-overridden remainder ---------- */

  .group-rest {
    align-items: center;
    display: flex;
    gap: var(--space-3);
    justify-content: space-between;
    /* Bleeds like the rows above it. Its separator is the last row's own
       bottom hairline, so the gaps around that line stay the row rhythm -
       and a card with nothing overridden shows no line under its title. */
    margin-inline: calc(var(--space-2) * -1);
    padding: var(--space-2) var(--space-2) 0;
    position: relative;
  }

  .rest-say {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    /* Ink-true, like the rows above it, so the air across the hairline
       between the last row and this line reads equal. */
    text-box: trim-both cap alphabetic;
  }

  .rest-count {
    color: var(--text-secondary);
    font-weight: 600;
  }

  .rest-picks {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    justify-content: flex-end;
  }

  /* On a phone the head's three parts cannot share one line - the tally or
     pill drops under the title instead of holding the card wide. */
  @media (max-width: 30rem) {
    .group-head {
      flex-wrap: wrap;
    }

    .group-rest {
      flex-wrap: wrap;
    }

    /* The say keeps the line and the control moves under it - beside it,
       the copy was down to a word a line while the control still ran off
       the screen and took the layout viewport with it. */
    .policy-row {
      grid-template-columns: minmax(0, 1fr) auto;
    }

    .policy-row .setting-say {
      grid-column: 1;
      grid-row: 1;
    }

    .policy-row .setting-clear {
      grid-column: 2;
      grid-row: 1;
      opacity: 1;
    }

    .policy-row .policy-value {
      flex-wrap: wrap;
      grid-column: 1 / -1;
      justify-self: start;
    }
  }
</style>

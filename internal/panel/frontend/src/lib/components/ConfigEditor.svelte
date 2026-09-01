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
  import Card from './Card.svelte';
  import Icon from './Icon.svelte';
  import Popover from './Popover.svelte';
  import Switch from './Switch.svelte';

  /* The linked-value rows name their inheritance source per scope. */
  const SOURCE_BY_SCOPE = {
    target: "Smyklot's defaults",
    repository: 'workspace defaults',
    runtime: 'the deployment',
  } as const;

  const {
    patch,
    inherited,
    scope,
    idPrefix,
    anchorPrefix,
    disabled = false,
    section = 'all',
    only,
    dirtyKeys = [],
    onChange,
  }: {
    patch: ConfigPatch;
    inherited: ConfigValues;
    scope: 'target' | 'repository' | 'runtime';
    idPrefix: string;
    /** Names the cards so a page index can link to them: `<prefix>-behavior`, `-commands`. */
    anchorPrefix?: string;
    disabled?: boolean;
    section?: 'all' | 'behavior' | 'commands';
    /** Render only these behavior rows. Used by the repository-file pane, which
     *  shows the overrides in effect rather than the whole settings list. */
    only?: readonly ConfigKey[];
    /** Keys whose draft values differ from their saved values. */
    dirtyKeys?: readonly ConfigKey[];
    /** Changes are staged synchronously and never saved by the editor. */
    onChange: (next: ConfigPatch, changedKey: ConfigKey) => void;
  } = $props();

  const source = $derived(SOURCE_BY_SCOPE[scope]);
  const shownFields = $derived(
    only === undefined
      ? BOOLEAN_FIELDS
      : BOOLEAN_FIELDS.filter((field) => only.includes(field.key)),
  );
  const dirtyKeySet = $derived(new Set(dirtyKeys));

  const initialPatch = clonePatch(untrack(() => patch));
  let draft = $state<ConfigPatch>(initialPatch);
  let receivedPatch = $state<ConfigPatch>(clonePatch(initialPatch));
  let picking = $state(false);
  let aliasOpen = $state(false);
  let aliasName = $state('');

  const editorDisabled = $derived(disabled);
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

  function report(changedKey: ConfigKey): void {
    onChange(clonePatch(draft), changedKey);
  }

  /* ---------- Behavior ---------- */

  function toggleBoolean(field: BooleanField, enabled: boolean): void {
    draft = setExplicitPatchValue(draft, field.key, fieldRawValue(field, enabled));
    report(field.key);
  }

  function clearField(key: ConfigKey): void {
    if (!Object.hasOwn(draft, key)) return;
    const next = { ...draft };
    delete next[key];
    draft = next;
    report(key);
  }

  /* Overriding pins what inheritance resolves to today; the switch beside it
     is how a different value is chosen. */
  function manage(field: BooleanField): void {
    draft = setExplicitPatchValue(draft, field.key, cloneValue(inherited[field.key]));
    picking = false;
    report(field.key);
  }

  /* ---------- Commands ---------- */

  function typePrefix(value: string): void {
    draft = updatePatchValue(draft, inherited, 'command_prefix', value);
    report('command_prefix');
  }

  function toggleCommand(command: string): void {
    const next = toggleAllowedCommand(allowedList, command, COMMANDS);
    draft = updatePatchValue(draft, inherited, 'allowed_commands', next);
    report('allowed_commands');
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
    report('command_aliases');
  }

  function retargetAlias(name: string, command: string): void {
    const current = effectiveValue(draft, inherited, 'command_aliases');
    draft = updatePatchValue(draft, inherited, 'command_aliases', {
      ...current,
      [name]: command,
    });
    report('command_aliases');
  }

  function removeAlias(name: string): void {
    const next = { ...effectiveValue(draft, inherited, 'command_aliases') };
    delete next[name];
    draft = updatePatchValue(draft, inherited, 'command_aliases', next);
    report('command_aliases');
  }

  function cloneValue<T>(value: T): T {
    return JSON.parse(JSON.stringify(value)) as T;
  }

  /**
   * What the unset settings are, said as scent rather than as a list.
   *
   * Every name spelled out ran the row's sentence to three lines and made the remainder
   * the loudest thing in the card; three names and a count says the same thing in one.
   */
  function scent(fields: readonly BooleanField[]): string {
    const names = fields.map((field) => field.label.toLowerCase());
    if (names.length <= 4) return `${names.slice(0, -1).join(', ')} and ${names.at(-1)}`;
    const rest = names.length - 3;
    return `${names.slice(0, 3).join(', ')}, and ${rest} ${rest === 1 ? 'other' : 'others'}`;
  }
</script>

<!--
@component
The settings a scope overrides, and what each would be if it did not. Every row shows
the inherited value beside the chosen one, so an override is always visibly a departure
rather than just a value.

`scope` decides what may be set at all - a repository can narrow what its workspace
allows and never widen it - and `only` renders a subset for a pane that shows a few
rows in another context.

The clear button is the way back to inherited, and it is what makes an override
reversible: without it a reader who overrides a value can never return to following the
account again.
-->

{#snippet resetButton(key: ConfigKey)}
  <!-- A WORD, NOT A GLYPH. The bare x read as "delete this setting" where it means
       "stop answering here and follow the account again". -->
  <Button
    tone="quiet"
    title="Stop overriding - follow {source}"
    disabled={editorDisabled}
    onclick={() => clearField(key)}>Reset</Button
  >
{/snippet}

<div class="config-editor card-stack">
  {#if section === 'all' || section === 'behavior'}
    {#if only !== undefined}
      <!-- The repository-file pane's list: just the rows in effect, no card of
           its own - the pane already stands inside one. -->
      <div class="policy-rows">
        {#each overriddenFields as field (field.key)}
          {@const on = fieldEnabled(field, effectiveValue(draft, inherited, field.key))}
          <div
            class={['policy-row', 'is-managed', { 'is-unsaved': dirtyKeySet.has(field.key) }]}
            data-unsaved={dirtyKeySet.has(field.key) || undefined}
          >
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
              {@render resetButton(field.key)}
            </span>
          </div>
        {/each}
      </div>
    {:else}
      <Card
        id={anchorPrefix === undefined ? undefined : `${anchorPrefix}-behavior`}
        labelledby="config-{scope}-{idPrefix}-behavior"
      >
        <div class="card-head">
          <h2 class="card-title" id="config-{scope}-{idPrefix}-behavior">Behavior</h2>
          <span class="card-meta">{overriddenFields.length} of {shownFields.length} set here</span>
        </div>
        <div class="policy-rows">
          {#each overriddenFields as field (field.key)}
            {@const on = fieldEnabled(field, effectiveValue(draft, inherited, field.key))}
            <div
              class={['policy-row', 'is-managed', { 'is-unsaved': dirtyKeySet.has(field.key) }]}
              data-unsaved={dirtyKeySet.has(field.key) || undefined}
            >
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
                {@render resetButton(field.key)}
              </span>
            </div>
          {/each}
          <!-- THE REMAINDER IS A ROW, not a footer under the list. It is one more thing
               this card has to say about the same nine settings, and said in a strip of
               its own it read as a second card's worth of chrome. -->
          {#if restFields.length > 0}
            <div class="policy-row" class:is-stacked={picking}>
              <span class="setting-say">
                <span class="setting-name"
                  >{restFields.length}
                  {overriddenFields.length === 0 ? '' : 'more '}follow {source}</span
                >
                <span class="setting-why">{scent(restFields)}</span>
              </span>
              {#if picking}
                <span class="policy-value setting-value-wrap">
                  {#each restFields as field (field.key)}
                    <button
                      class="add-chip"
                      disabled={editorDisabled}
                      onclick={() => manage(field)}
                    >
                      <Icon name="plus" size="xs" />
                      <span class="t">{field.label}</span>
                    </button>
                  {/each}
                  <Button tone="quiet" onclick={() => (picking = false)}>Cancel</Button>
                </span>
              {:else}
                <span class="policy-value">
                  <Button tone="quiet" disabled={editorDisabled} onclick={() => (picking = true)}>
                    {#snippet icon()}<Icon name="plus" size="sm" />{/snippet}
                    Override another
                  </Button>
                </span>
              {/if}
            </div>
          {/if}
        </div>
      </Card>
    {/if}
  {/if}

  {#if only === undefined && (section === 'all' || section === 'commands')}
    <Card
      id={anchorPrefix === undefined ? undefined : `${anchorPrefix}-commands`}
      labelledby="config-{scope}-{idPrefix}-commands"
    >
      <div class="card-head">
        <h2 class="card-title" id="config-{scope}-{idPrefix}-commands">Commands</h2>
        <span class="card-meta">{commandsOverridden} of 3 set here</span>
      </div>
      <div class="policy-rows">
        <div
          class={[
            'policy-row',
            { 'is-managed': Object.hasOwn(draft, 'command_prefix') },
            { 'is-unsaved': dirtyKeySet.has('command_prefix') },
          ]}
          data-unsaved={dirtyKeySet.has('command_prefix') || undefined}
        >
          <span class="setting-say">
            <label class="setting-name" for="config-{scope}-{idPrefix}-prefix">Prefix</label>
            <span class="setting-why"
              >What a comment starts with to address Smyklot. Editing the inherited value creates an
              override</span
            >
          </span>
          <span class="policy-value">
            <input
              id="config-{scope}-{idPrefix}-prefix"
              class="prefix-inline"
              value={effectiveValue(draft, inherited, 'command_prefix')}
              {disabled}
              oninput={(event) => typePrefix(event.currentTarget.value)}
            />
            {#if Object.hasOwn(draft, 'command_prefix')}
              {@render resetButton('command_prefix')}
            {/if}
          </span>
        </div>

        <!-- A CHECKLIST, NOT A VALUE AT THE END OF A LINE. The command set is a set to
             read across, so the row is authored as two lines rather than computed into
             them at the width where it finally does not fit. -->
        <div
          class={[
            'policy-row',
            { 'is-managed': Object.hasOwn(draft, 'allowed_commands') },
            { 'is-unsaved': dirtyKeySet.has('allowed_commands') },
          ]}
          data-unsaved={dirtyKeySet.has('allowed_commands') || undefined}
        >
          <span class="setting-say">
            <span class="setting-name" id="config-{scope}-{idPrefix}-allowed"
              >Commands it answers</span
            >
            <span class="setting-why"
              >A command turned off here is refused, with a comment saying so. At least one must
              remain on</span
            >
          </span>
          <span class="policy-value">
            <span
              class="check-line"
              role="group"
              aria-labelledby="config-{scope}-{idPrefix}-allowed"
            >
              {#each COMMANDS as command (command)}
                {@const on = commandIsAllowed(allowedList, command)}
                <label class="check-item">
                  <input
                    type="checkbox"
                    checked={on}
                    disabled={editorDisabled || (on && allowedCount === 1)}
                    onchange={() => toggleCommand(command)}
                  />
                  <span class="check-box"><Icon name="check" size="micro" /></span>
                  <span class="check-word">{command}</span>
                </label>
              {/each}
            </span>
            {#if Object.hasOwn(draft, 'allowed_commands')}
              {@render resetButton('allowed_commands')}
            {/if}
          </span>
        </div>

        <div
          class={[
            'policy-row',
            { 'is-managed': Object.hasOwn(draft, 'command_aliases') },
            { 'is-unsaved': dirtyKeySet.has('command_aliases') },
          ]}
          data-unsaved={dirtyKeySet.has('command_aliases') || undefined}
        >
          <span class="setting-say">
            <span class="setting-name" id="config-{scope}-{idPrefix}-aliases">Aliases</span>
            <span class="setting-why">Extra words mapped to the commands they run</span>
          </span>
          <span class="policy-value setting-value-wrap">
            <span
              class="chip-line"
              role="group"
              aria-labelledby="config-{scope}-{idPrefix}-aliases"
            >
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
                        aria-label="{command} - the command {name} invokes"
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
                            {#if candidate === command}<Icon name="check" size="base" />{/if}
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
                    <Icon name="close" size="nano" />
                  </button>
                </span>
              {/each}
              <Popover role="dialog" label="Name the alias" align="start" bind:open={aliasOpen}>
                {#snippet trigger(attributes)}
                  <button {...attributes} class="add-chip" disabled={editorDisabled}>
                    <Icon name="plus" size="xs" />
                    <span class="t">Add an alias</span>
                  </button>
                {/snippet}
                <div class="name-menu">
                  <div class="menu-search">
                    <Icon name="search" size="xs" />
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
                    {aliasTaken
                      ? 'That word is taken'
                      : 'Enter adds it as approve · retarget after'}
                  </div>
                </div>
              </Popover>
            </span>
            {#if Object.hasOwn(draft, 'command_aliases')}
              {@render resetButton('command_aliases')}
            {/if}
          </span>
        </div>
      </div>
    </Card>
  {/if}
</div>

<style>
  .group-tally {
    color: var(--text-muted);
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    min-block-size: 8px;
    text-box: trim-both cap alphabetic;
  }

  /* ---------- The command rows' own controls ---------- */

  .prefix-inline {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text-primary);
    font-family: var(--mono);
    font-size: var(--font-size-control);
    min-block-size: var(--tier-quiet);
    padding: 0;
    text-align: center;
    width: 4.5rem;
  }

  .prefix-inline:focus-visible {
    border-color: var(--brand-action);
    outline: 2px solid var(--focus);
  }

  /* A block row keeps its sentence and its count on the first line and lays the chips on
     a full-width second one. `flex-basis: 100%` is what takes that line under the row
     law - the old `grid-column: 1 / -1` addressed a grid the row no longer is. */
  .chip-line {
    align-items: center;
    display: flex;
    flex-basis: 100%;
    flex-wrap: wrap;
    gap: var(--space-2);
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
    line-height: var(--leading-flat);
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
    line-height: var(--leading-flat);
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
    line-height: var(--leading-tight);
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
  }
</style>

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
  } from '../config';
  import type { BooleanField } from '../config';
  import { COMMANDS } from '../types';
  import type { ConfigKey, ConfigPatch, ConfigValues } from '../types';
  import Select from './Select.svelte';
  import ChangedMarker from './ChangedMarker.svelte';
  import AliasChip from './AliasChip.svelte';
  import CheckTile from './CheckTile.svelte';
  import SaveBar from './SaveBar.svelte';
  import HelpTip from './HelpTip.svelte';
  import Icon from './Icon.svelte';
  import InheritControl from './InheritControl.svelte';

  const VALUE_OPTIONS = [
    { value: 'enabled', label: 'Enabled' },
    { value: 'disabled', label: 'Disabled' },
  ] as const;
  const CUSTOM_OPTIONS = [{ value: 'custom', label: 'Custom' }] as const;
  /* The linked-value control names its inheritance source per scope. */
  const SOURCE_BY_SCOPE = {
    target: 'the application defaults',
    repository: 'workspace defaults',
    runtime: 'the deployment configuration',
  } as const;

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
    onSave: (next: ConfigPatch) => Promise<void>;
  } = $props();

  const shownFields = $derived(
    only === undefined
      ? BOOLEAN_FIELDS
      : BOOLEAN_FIELDS.filter((field) => only.includes(field.key)),
  );

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
    draft = setExplicitPatchValue(draft, field.key, fieldRawValue(field, selection === 'enabled'));
  }

  function booleanOverrideValue(field: BooleanField): string | null {
    if (!hasOverride(field.key)) return null;
    const raw = effectiveValue(draft, inherited, field.key);
    return fieldEnabled(field, raw) ? 'enabled' : 'disabled';
  }

  function inheritedBooleanValue(field: BooleanField): string {
    return fieldEnabled(field, inherited[field.key]) ? 'enabled' : 'disabled';
  }

  function inheritedAllowedLabel(): string {
    const list = inherited.allowed_commands;
    const count = list.length === 0 ? COMMANDS.length : list.length;
    return `${count} command${count === 1 ? '' : 's'}`;
  }

  function inheritedAliasLabel(): string {
    const count = Object.keys(inherited.command_aliases).length;
    return count === 0 ? 'no aliases' : `${count} alias${count === 1 ? '' : 'es'}`;
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

      <div class="rows rows-plain">
        {#each shownFields as field (field.key)}
          {@const changed = keyChanged(field.key)}
          <div class="row" class:changed>
            <span class="row-label">
              <!-- Inside the label, not beside it: the marker belongs to the
                   name it marks, so it rides the label's own 0.45rem gap
                   instead of the row's wider one. -->
              {#if changed}
                <ChangedMarker />
              {/if}
              <span class="label-text band-trim">{field.label}</span>
              <HelpTip
                id="config-{scope}-{idPrefix}-{field.key}-tooltip"
                label="About {field.label.toLowerCase()}"
                text={field.help}
                align="start"
              />
            </span>
            <span class="visually-hidden" id="config-{scope}-{idPrefix}-{field.key}-help">
              {field.help}
            </span>
            <span class="row-spacer"></span>
            <InheritControl
              label={field.label}
              source={SOURCE_BY_SCOPE[scope]}
              sourcePronoun={scope === 'repository' ? 'them' : 'it'}
              inheritedValue={inheritedBooleanValue(field)}
              inheritedLabel={inheritedBooleanValue(field) === 'enabled' ? 'Enabled' : 'Disabled'}
              value={booleanOverrideValue(field)}
              options={VALUE_OPTIONS}
              disabled={editorDisabled}
              onSelect={(selection) => selectBoolean(field, selection)}
              onRestore={() => useDefault(field.key)}
            />
          </div>
        {/each}
      </div>
    </section>
  {/if}

  {#if only === undefined && (section === 'all' || section === 'commands')}
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
        <div class="row-group" class:changed={keyChanged('command_prefix')}>
          <div class="row-line">
            <span class="row-label">
              <label class="band-trim" for="config-{scope}-{idPrefix}-prefix">Prefix</label>
              <HelpTip
                id="config-{scope}-{idPrefix}-prefix-tooltip"
                label="About the command prefix"
                text="Characters required before a command when prefix invocation is used. Editing the {scope ===
                'target'
                  ? 'built-in default'
                  : 'inherited value'} creates an override"
                align="start"
              />
            </span>
            <span class="row-spacer"></span>
            {#if keyChanged('command_prefix')}
              <ChangedMarker />
            {/if}
            <input
              id="config-{scope}-{idPrefix}-prefix"
              class="prefix-input mono"
              value={effectiveValue(draft, inherited, 'command_prefix')}
              disabled={editorDisabled}
              oninput={(event) => setPrefix(event.currentTarget.value)}
            />
            <InheritControl
              label="Command prefix source"
              source={SOURCE_BY_SCOPE[scope]}
              sourcePronoun={scope === 'repository' ? 'them' : 'it'}
              inheritedLabel={`"${inherited.command_prefix}"`}
              value={hasOverride('command_prefix') ? 'custom' : null}
              options={CUSTOM_OPTIONS}
              disabled={editorDisabled}
              onSelect={() => useCustom('command_prefix')}
              onRestore={() => useDefault('command_prefix')}
            />
          </div>
        </div>

        <div class="row-group" class:changed={keyChanged('allowed_commands')}>
          <div class="row-line">
            <span class="row-label">
              <span class="label-text band-trim">Allowed commands</span>
              <HelpTip
                id="config-{scope}-{idPrefix}-commands-tooltip"
                label="About allowed commands"
                text="The command words Smyklot accepts. At least one must remain enabled. Editing the selection creates an override"
                align="start"
              />
            </span>
            <span class="row-spacer"></span>
            {#if keyChanged('allowed_commands')}
              <ChangedMarker />
            {/if}
            <InheritControl
              label="Allowed commands source"
              source={SOURCE_BY_SCOPE[scope]}
              sourcePronoun={scope === 'repository' ? 'them' : 'it'}
              inheritedLabel={inheritedAllowedLabel()}
              value={hasOverride('allowed_commands') ? 'custom' : null}
              options={CUSTOM_OPTIONS}
              disabled={editorDisabled}
              onSelect={() => useCustom('allowed_commands')}
              onRestore={() => useDefault('allowed_commands')}
            />
          </div>
          <div class="row-body">
            <div class="cmd-flow">
              {#each COMMANDS as command (command)}
                {@const checked = commandIsAllowed(allowedList, command)}
                <CheckTile
                  label={command}
                  {checked}
                  disabled={editorDisabled || (checked && allowedCount === 1)}
                  onchange={() => toggleCommand(command)}
                />
              {/each}
            </div>
          </div>
        </div>

        <div class="row-group" class:changed={keyChanged('command_aliases')}>
          <div class="row-line">
            <span class="row-label" id="config-{scope}-{idPrefix}-aliases-heading">
              <span class="label-text band-trim">Aliases</span>
              <HelpTip
                id="config-{scope}-{idPrefix}-aliases-tooltip"
                label="About command aliases"
                text="Extra command words mapped to canonical commands. Changing an alias creates an override"
                align="start"
              />
            </span>
            <span class="row-spacer"></span>
            {#if keyChanged('command_aliases')}
              <ChangedMarker />
            {/if}
            <InheritControl
              label="Command aliases source"
              source={SOURCE_BY_SCOPE[scope]}
              sourcePronoun={scope === 'repository' ? 'them' : 'it'}
              inheritedLabel={inheritedAliasLabel()}
              value={hasOverride('command_aliases') ? 'custom' : null}
              options={CUSTOM_OPTIONS}
              disabled={editorDisabled}
              onSelect={() => useCustom('command_aliases')}
              onRestore={() => {
                useDefault('command_aliases');
                closeComposer();
              }}
            />
          </div>
          <div
            class="row-body alias-flow"
            role="group"
            aria-labelledby="config-{scope}-{idPrefix}-aliases-heading"
          >
            {#each aliasEntries as [name, command] (name)}
              <AliasChip
                from={name}
                to={command}
                added={savedAliases[name] !== command}
                disabled={editorDisabled}
                onRemove={() => removeAlias(name)}
              />
            {:else}
              <span class="alias-empty band-trim">No aliases yet</span>
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
                <Select
                  id="config-{scope}-{idPrefix}-alias-command"
                  class="mono"
                  bind:value={aliasCommand}
                  disabled={editorDisabled}
                >
                  {#each COMMANDS as command (command)}
                    <option value={command}>{command}</option>
                  {/each}
                </Select>
                <button
                  type="submit"
                  class="composer-ok band-trim"
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
                <span class="band-trim">Add alias</span>
              </button>
            {/if}
          </div>
        </div>
      </div>
    </section>
  {/if}

  {#if dirty}
    <SaveBar
      count={changedKeys.length}
      {saving}
      disabled={editorDisabled}
      inline={scope === 'repository'}
      onSave={save}
      onDiscard={discard}
    />
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

  /* Behavior rows sit on inset hairlines that follow the plate's content padding —
     no outer box, and the separators never run full-bleed. */
  .rows-plain {
    border: 0;
    border-radius: 0;
  }

  .rows-plain > :first-child,
  .rows-plain > :last-child {
    border-radius: 0;
  }

  .rows-plain .row {
    gap: var(--space-3);
    min-height: 0;
    padding-block: 0.7rem;
    padding-inline: 0;
  }

  .rows-plain > .row:first-child {
    padding-top: 0.15rem;
  }

  .rows-plain > .row:last-child {
    padding-bottom: 0.15rem;
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
    gap: 0.45rem;
  }

  .row-label label {
    cursor: pointer;
  }

  .row-spacer {
    flex: 1;
  }

  /* Unsaved reads as a warning, not as information: the mock tints the row and
     rings the marker in --warning, and an unsaved edit is something you are
     being asked to resolve. */
  .row.changed,
  .row-group.changed {
    background: color-mix(in srgb, var(--warning) 4%, var(--surface-base));
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

  .alias-flow {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .alias-empty {
    color: var(--dim);
    font-size: var(--font-size-meta);
  }

  /* `.add-chip` is in `app.css` and this restated it, height included - so
     raising the shared control left this one page behind at 24px beside the
     34px chip it adds to. The local copy said in a comment that it was keeping
     the shared height, which is exactly what a second copy cannot do. */
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
  .composer :global(select) {
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

  .composer :global(select) {
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

  /* Inline, the bar is not a floating slab: no vertical padding of its own, the
     approved 12px gap, and the trailing inset that lines its Save up with the
     rows' right edge. */

  /* The inline bar carries no status dot: it sits directly under the row it
     belongs to, and the row already has its unsaved marker. */

  /* Regular weight inline: the count is a sentence under the row, not a label
     on a dark slab where 600 is what keeps it legible. */

  /* Full-strength text, like the mock's ghost button - a Discard that reads as
     disabled is a Discard nobody dares press. It also wears the button's own
     box here, so it stands the same 34px as the Save beside it. */

  /* On a phone the row's parts do not fit on one line. The control holds a fixed
     width - a segmented control does not shrink - and the label is the only part
     that gives, so it collapsed to a one-word column while the control still ran
     past the screen and took the page's layout viewport with it: Chrome widens
     the viewport to fit the overflow and zooms the whole page out to compensate,
     so one row too wide shrank every glyph on the page.

     Wrapped rather than stacked with `flex-direction`, because the prefix row's
     input and its control still belong on one line together; it is only the
     label that needs the width to itself. */
  @media (max-width: 30rem) {
    .row,
    .row-line {
      flex-wrap: wrap;
    }

    .row-label {
      flex: 1 0 100%;
    }

    /* It exists to push the control to the far end of a shared line. On its own
       line there is no far end, and the control reads as belonging to the label
       above it only if it starts where the label starts. */
    .row-spacer {
      display: none;
    }
  }

  @media (prefers-reduced-motion: reduce) {
  }
</style>

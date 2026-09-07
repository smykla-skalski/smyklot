<script lang="ts">
  import { onDestroy, untrack } from 'svelte';

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
  import PairEntry from './PairEntry.svelte';
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
    onValidity = () => {},
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
    onValidity?: (problem: string | null) => void;
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
  let addingAlias = $state(false);
  let aliasProblems = $state<Record<string, string | null>>({});
  const commandOptions = [
    { value: 'approve', label: 'approve', description: 'Approves the pull request' },
    { value: 'merge', label: 'merge', description: 'Merges when checks pass' },
    { value: 'squash', label: 'squash', description: 'Squashes and merges' },
    { value: 'rebase', label: 'rebase', description: 'Rebases and merges' },
    { value: 'unapprove', label: 'unapprove', description: 'Withdraws the approval' },
    { value: 'cleanup', label: 'cleanup', description: 'Deletes the merged branch' },
    { value: 'help', label: 'help', description: 'Lists available commands' },
  ];

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
  $effect(() =>
    onValidity(Object.values(aliasProblems).find((problem) => problem !== null) ?? null),
  );
  onDestroy(() => onValidity(null));

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
    if (key === 'command_aliases') {
      addingAlias = false;
      aliasProblems = {};
    }
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

  function aliasProblem(name: string, previousName?: string): string | null {
    if (!/^[A-Za-z0-9_]{1,64}$/u.test(name)) {
      return 'Use 1 to 64 letters, numbers or underscores';
    }
    if (
      name !== previousName &&
      Object.hasOwn(effectiveValue(draft, inherited, 'command_aliases'), name)
    ) {
      return 'That alias already exists';
    }
    return null;
  }

  function saveAlias(previousName: string | null, name: string, command: string): void {
    if (aliasProblem(name, previousName ?? undefined) !== null) return;
    const next = { ...effectiveValue(draft, inherited, 'command_aliases') };
    if (previousName !== null) delete next[previousName];
    next[name] = command;
    delete aliasProblems[previousName ?? ''];
    draft = updatePatchValue(draft, inherited, 'command_aliases', next);
    addingAlias = false;
    report('command_aliases');
  }

  function removeAlias(name: string): void {
    const next = { ...effectiveValue(draft, inherited, 'command_aliases') };
    delete next[name];
    delete aliasProblems[name];
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
                    <Button tone="quiet" disabled={editorDisabled} onclick={() => manage(field)}>
                      {#snippet icon()}<Icon name="plus" size="sm" />{/snippet}
                      {field.label}
                    </Button>
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
              class="text-input mono prefix-inline"
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
            <span class="command-heading">
              <span class="setting-name" id="config-{scope}-{idPrefix}-allowed"
                >Commands it answers</span
              >
              {#if Object.hasOwn(draft, 'allowed_commands')}
                <span class="command-reset">{@render resetButton('allowed_commands')}</span>
              {/if}
            </span>
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
          <span class="policy-value alias-controls">
            <span
              class="alias-pairs"
              role="group"
              aria-labelledby="config-{scope}-{idPrefix}-aliases"
            >
              {#each aliasEntries as [name, command] (name)}
                <PairEntry
                  keyValue={name}
                  value={command}
                  keyLabel="Alias {name}"
                  valueLabel="Command for alias {name}"
                  removeLabel="Remove alias {name}"
                  options={commandOptions}
                  disabled={editorDisabled}
                  validateKey={(next) => aliasProblem(next, name)}
                  onCommit={(next, target) => saveAlias(name, next, target)}
                  onRemove={() => removeAlias(name)}
                  onProblem={(problem) => (aliasProblems[name] = problem)}
                />
              {/each}
              {#if addingAlias}
                <PairEntry
                  keyValue=""
                  value=""
                  keyLabel="Name for the new alias"
                  valueLabel="Command for the new alias"
                  removeLabel="Discard this alias"
                  options={commandOptions}
                  disabled={editorDisabled}
                  draft
                  focusOnMount
                  validateKey={(next) => aliasProblem(next)}
                  onCommit={(name, command) => saveAlias(null, name, command)}
                  onRemove={() => {
                    addingAlias = false;
                    delete aliasProblems[''];
                  }}
                  onProblem={(problem) => (aliasProblems[''] = problem)}
                />
              {/if}
              <Button
                tone="quiet"
                disabled={editorDisabled || addingAlias}
                onclick={() => (addingAlias = true)}
              >
                {#snippet icon()}<Icon name="plus" size="xs" />{/snippet}
                Add an alias
              </Button>
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
  /* ---------- The command rows' own controls ---------- */

  .prefix-inline {
    text-align: center;
    width: 4.5rem;
  }

  .command-heading {
    align-items: center;
    display: flex;
    justify-content: space-between;
    gap: var(--space-2);
  }
  .command-heading + .setting-why {
    margin-block-start: var(--row-copy-gap);
  }
  .command-heading .command-reset {
    margin-block: calc((10px - var(--control-height-compact)) / 2);
  }
  .alias-controls {
    align-items: flex-start;
    flex-wrap: nowrap;
  }
  .alias-pairs {
    align-items: center;
    display: flex;
    flex: 1 1 auto;
    flex-wrap: wrap;
    gap: var(--space-2);
    justify-content: flex-end;
    min-inline-size: 0;
  }
  .command-reset {
    flex: none;
  }
  @container (max-width: 25rem) {
    .alias-pairs {
      justify-content: flex-start;
    }
  }
</style>

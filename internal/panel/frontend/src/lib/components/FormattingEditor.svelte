<script lang="ts">
  import { onDestroy, untrack } from 'svelte';

  import {
    FORMATTING_FIELDS,
    FORMATTING_GROUPS,
    FORMATTING_PRESETS,
    applyFormattingPatch,
    cloneFormattingPatch,
    formattingOverrideCount,
    formattingPatchValue,
    formattingPatchesEqual,
    formattingPolicyValue,
    setFormattingPatchValue,
    type FormattingField,
    type FormattingFieldKey,
    type FormattingPatch,
    type FormattingPolicy,
  } from '../formatting';
  import AppTooltip from './AppTooltip.svelte';
  import Card from './Card.svelte';
  import Icon from './Icon.svelte';
  import InheritControl from './InheritControl.svelte';

  /* The level a value comes from, named as a place rather than as a document: the
     dictionary allows "from the workspace" and "from the service", and the four
     phrases this used to carry - "the application defaults", "workspace defaults",
     "the deployment configuration" - are the words it retires. */
  const SOURCE_BY_SCOPE = {
    target: 'the service',
    repository: 'the workspace',
    runtime: 'the deployment',
    template: 'the workspace',
    path: 'the repository',
  } as const;

  const {
    patch,
    inherited,
    scope,
    idPrefix,
    anchor,
    disabled = false,
    dirtyKeys = [],
    onChange,
    onValidity = () => {},
  }: {
    patch: FormattingPatch;
    inherited: FormattingPolicy;
    scope: keyof typeof SOURCE_BY_SCOPE;
    idPrefix: string;
    /** Names the first card, so a page index can link to where the editor starts. */
    anchor?: string;
    disabled?: boolean;
    dirtyKeys?: readonly FormattingFieldKey[];
    onChange: (next: FormattingPatch, changedKey: FormattingFieldKey) => void;
    onValidity?: (valid: boolean) => void;
  } = $props();

  const initialPatch = cloneFormattingPatch(untrack(() => patch));
  let draft = $state<FormattingPatch>(initialPatch);
  let receivedPatch = $state<FormattingPatch>(cloneFormattingPatch(initialPatch));
  let numberDrafts = $state<Record<string, string>>({});
  let invalidNumbers = $state<Record<string, true>>({});

  const source = $derived(SOURCE_BY_SCOPE[scope]);
  const dirtyKeySet = $derived(new Set(dirtyKeys));
  const valid = $derived(Object.keys(invalidNumbers).length === 0);
  const baseline = $derived(
    draft.preset === undefined ? inherited : FORMATTING_PRESETS[draft.preset],
  );
  const effective = $derived(applyFormattingPatch(inherited, draft));
  const presetField = FORMATTING_FIELDS[0];
  const overridden = $derived(formattingOverrideCount(draft));

  $effect(() => {
    const incoming = cloneFormattingPatch(patch);
    if (formattingPatchesEqual(receivedPatch, incoming)) return;
    receivedPatch = incoming;
    draft = incoming;
    numberDrafts = {};
    invalidNumbers = {};
  });

  $effect(() => {
    const next = valid;
    untrack(() => onValidity(next));
  });

  onDestroy(() => onValidity(true));

  function fieldsIn(group: string): readonly FormattingField[] {
    return FORMATTING_FIELDS.filter((field) => field.path[0] === group);
  }

  function report(field: FormattingField, value: string | number | undefined): void {
    draft = setFormattingPatchValue(draft, field, value);
    onChange(cloneFormattingPatch(draft), field.key);
  }

  function pick(field: FormattingField, value: string): void {
    report(field, value);
  }

  function clear(field: FormattingField): void {
    const nextDrafts = { ...numberDrafts };
    const nextInvalid = { ...invalidNumbers };
    delete nextDrafts[field.key];
    delete nextInvalid[field.key];
    numberDrafts = nextDrafts;
    invalidNumbers = nextInvalid;
    report(field, undefined);
  }

  function typeNumber(field: FormattingField & { kind: 'int' }, raw: string): void {
    numberDrafts = { ...numberDrafts, [field.key]: raw };
    const value = Number(raw);
    const validValue =
      raw.trim() !== '' &&
      Number.isSafeInteger(value) &&
      value >= field.minimum &&
      value <= field.maximum;
    const nextInvalid = { ...invalidNumbers };
    if (!validValue) {
      nextInvalid[field.key] = true;
      invalidNumbers = nextInvalid;
      return;
    }
    delete nextInvalid[field.key];
    invalidNumbers = nextInvalid;
    report(field, value);
  }

  function finishNumber(field: FormattingField & { kind: 'int' }): void {
    if (invalidNumbers[field.key]) return;
    const next = { ...numberDrafts };
    delete next[field.key];
    numberDrafts = next;
  }

  function shownNumber(field: FormattingField & { kind: 'int' }): string {
    return (
      numberDrafts[field.key] ??
      String(formattingPatchValue(draft, field) ?? formattingPolicyValue(baseline, field))
    );
  }

  function optionLabel(value: string): string {
    return value
      .split('_')
      .map((word) => `${word.charAt(0).toUpperCase()}${word.slice(1)}`)
      .join(' ');
  }

  function fieldLabel(field: FormattingField): string {
    return optionLabel(field.path.at(-1) ?? field.key);
  }
</script>

<!--
@component
How a managed file is written: quoting, indentation, line endings, the order of keys.
Every field shows what it would inherit and what this scope overrides, so a reader can
always see which of the two they are looking at.

It reports validity out through `onValidity` rather than blocking. A formatting patch
that cannot be applied is the save composer's business to refuse - this editor's job is
to say which field is wrong and why, next to the field.

`dirtyKeys` marks what this draft has touched, which is how a reader who has changed
three things among thirty finds them again.
-->

<div class="formatting-editor card-stack" data-valid={valid}>
  <Card id={anchor} labelledby="formatting-{scope}-{idPrefix}-policy">
    <div class="card-head">
      <h2 class="card-title" id="formatting-{scope}-{idPrefix}-policy">Formatting</h2>
      <span class="card-meta">{overridden} of {FORMATTING_FIELDS.length} set here</span>
    </div>
    <p class="group-note">Presentation rules applied after semantic file merges</p>
    <div
      class={['policy-row', { 'is-unsaved': dirtyKeySet.has(presetField.key) }]}
      data-unsaved={dirtyKeySet.has(presetField.key) || undefined}
    >
      <span class="setting-say">
        <span class="setting-name">Preset</span>
        <span class="setting-why">{presetField.description}</span>
      </span>
      <!-- `fluid` fits the segments to the column they are in, and the column is the row
           law's own half - so the control never sets the page's width. Without it a
           segment's longest word decided the document at 320px. -->
      <span class="policy-value">
        <InheritControl
          label="Formatting preset"
          {source}
          inheritedValue={inherited.preset}
          inheritedLabel={optionLabel(inherited.preset)}
          value={draft.preset ?? null}
          options={presetField.options.map((value) => ({ value, label: optionLabel(value) }))}
          {disabled}
          fluid
          onSelect={(value) => pick(presetField, value)}
          onRestore={() => clear(presetField)}
        />
      </span>
    </div>
  </Card>

  {#each FORMATTING_GROUPS as group (group.key)}
    <Card labelledby="formatting-{scope}-{idPrefix}-{group.key}">
      <div class="card-head">
        <h2 class="card-title" id="formatting-{scope}-{idPrefix}-{group.key}">{group.label}</h2>
        <span class="card-meta"
          >{fieldsIn(group.key).filter((field) => formattingPatchValue(draft, field) !== undefined)
            .length} of {fieldsIn(group.key).length} set here</span
        >
      </div>
      <p class="group-note">{group.description}</p>
      <div class="policy-rows">
        {#each fieldsIn(group.key) as field (field.key)}
          <div
            class={['policy-row', { 'is-unsaved': dirtyKeySet.has(field.key) }]}
            data-unsaved={dirtyKeySet.has(field.key) || undefined}
          >
            <span class="setting-say">
              <label class="setting-name" for="formatting-{scope}-{idPrefix}-{field.key}"
                >{fieldLabel(field)}</label
              >
              <span class="setting-why">{field.description}</span>
            </span>
            {#if field.kind === 'enum'}
              <span class="policy-value">
                <InheritControl
                  label={fieldLabel(field)}
                  {source}
                  inheritedValue={String(formattingPolicyValue(baseline, field))}
                  inheritedLabel={optionLabel(String(formattingPolicyValue(baseline, field)))}
                  value={formattingPatchValue(draft, field)?.toString() ?? null}
                  options={field.options.map((value) => ({ value, label: optionLabel(value) }))}
                  {disabled}
                  fluid
                  onSelect={(value) => pick(field, value)}
                  onRestore={() => clear(field)}
                />
              </span>
            {:else}
              <span class="policy-value number-control">
                {#if formattingPatchValue(draft, field) !== undefined}
                  <AppTooltip text="Stop overriding - take the value from {source}">
                    {#snippet children(attributes)}
                      <button
                        {...attributes}
                        type="button"
                        class="link-toggle broken"
                        aria-label="Stop overriding {fieldLabel(field)}"
                        {disabled}
                        onclick={() => clear(field)}
                      >
                        <Icon name="link-off" size="sm" strokeWidth={2} />
                      </button>
                    {/snippet}
                  </AppTooltip>
                {:else}
                  <!-- NAMED, because it is focusable. A tooltip trigger takes the
                       keyboard - that is how a tooltip is reached without a pointer - so
                       this mark is a stop on the tab ring, and an unnamed stop is a stop
                       that announces nothing when a reader arrives at it. The name is
                       what the tooltip says, because that is what the mark means. -->
                  <AppTooltip text="From {source}: {formattingPolicyValue(baseline, field)}">
                    {#snippet children(attributes)}
                      <span
                        {...attributes}
                        class="link-toggle"
                        role="note"
                        aria-label="{fieldLabel(field)} comes from {source}"
                      >
                        <Icon name="link" size="sm" strokeWidth={2} />
                      </span>
                    {/snippet}
                  </AppTooltip>
                {/if}
                <input
                  id="formatting-{scope}-{idPrefix}-{field.key}"
                  class="number-input"
                  type="number"
                  inputmode="numeric"
                  min={field.minimum}
                  max={field.maximum}
                  step="1"
                  value={shownNumber(field)}
                  aria-invalid={invalidNumbers[field.key] || undefined}
                  aria-describedby={invalidNumbers[field.key]
                    ? `formatting-${scope}-${idPrefix}-${field.key}-error`
                    : undefined}
                  {disabled}
                  oninput={(event) => typeNumber(field, event.currentTarget.value)}
                  onblur={() => finishNumber(field)}
                />
                {#if invalidNumbers[field.key]}
                  <span
                    class="field-error"
                    id="formatting-{scope}-{idPrefix}-{field.key}-error"
                    role="alert">Use a whole number from {field.minimum} to {field.maximum}</span
                  >
                {/if}
              </span>
            {/if}
          </div>
        {/each}
      </div>
    </Card>
  {/each}

  <span class="effective-summary" aria-live="polite">
    Effective preset: {optionLabel(effective.preset)}
  </span>
</div>

<style>
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
  }

  /* `.group-note` is the sheet's - it is a card's note wherever it appears. */
  .group-tally,
  .effective-summary {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
  }

  .group-tally {
    white-space: nowrap;
  }

  .number-control {
    align-items: center;
    display: grid;
    gap: var(--space-2);
    grid-template-columns: var(--inherit-marker-size) 6rem;
  }

  .number-input {
    background: var(--control-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text-primary);
    font: inherit;
    min-block-size: 30px;
    padding-inline: var(--space-2);
    width: 6rem;
  }

  .number-input[aria-invalid='true'] {
    border-color: var(--danger);
    outline: 1px solid var(--danger);
  }

  .link-toggle {
    background: transparent;
    border: 0;
    border-radius: 6px;
    color: var(--text-muted);
    display: grid;
    height: var(--inherit-marker-size);
    place-items: center;
    width: var(--inherit-marker-size);
  }

  button.link-toggle {
    color: var(--warning);
    cursor: pointer;
  }

  button.link-toggle:hover {
    background: var(--surface-inset);
  }

  .field-error {
    color: var(--danger);
    font-size: var(--font-size-micro);
    grid-column: 1 / -1;
    text-align: end;
  }

  .effective-summary {
    justify-self: end;
  }

  @media (max-width: 30rem) {
    .card {
      padding: var(--space-3);
    }

    .group-head {
      flex-wrap: wrap;
    }
  }
</style>

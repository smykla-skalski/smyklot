<script lang="ts">
  import { onDestroy, untrack } from 'svelte';

  import {
    FORMATTING_FIELDS,
    FORMATTING_GROUPS,
    FORMATTING_PRESETS,
    applyFormattingPatch,
    cloneFormattingPatch,
    formattingPatchValue,
    formattingPatchesEqual,
    formattingPolicyValue,
    formattingSourceValue,
    setFormattingPatchValue,
    type FormattingField,
    type FormattingFieldKey,
    type FormattingPatch,
    type FormattingPolicy,
    type FormattingSources,
  } from '../formatting';
  import type { SyncFileFormattingResolution } from '../sync-file-render.generated';
  import AppTooltip from './AppTooltip.svelte';
  import Icon from './Icon.svelte';
  import InheritControl from './InheritControl.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  const SOURCE_BY_SCOPE = {
    target: 'Global',
    repository: 'Organization',
    runtime: 'Deployment',
    template: 'Organization',
    path: 'Template or repository',
  } as const;

  const SOURCE_LABEL: Record<string, string> = {
    process: 'Global',
    target: 'Organization',
    repository_file: 'Repository config',
    repository_panel: 'Repository panel',
    template: 'Template',
    repository_path: 'File override',
  };

  const LAYERS_BY_SCOPE = {
    runtime: ['Deployment', 'Global'],
    target: ['Global', 'Organization'],
    repository: ['Global', 'Organization', 'Repository config', 'Repository panel'],
    template: [],
    path: [],
  } as const;

  const {
    patch,
    inherited,
    scope,
    idPrefix,
    path,
    sources,
    resolution,
    disabled = false,
    dirtyKeys = [],
    onChange,
    onValidity = () => {},
  }: {
    patch: FormattingPatch;
    inherited: FormattingPolicy;
    scope: keyof typeof SOURCE_BY_SCOPE;
    idPrefix: string;
    /** Restricts file editing to common rules and this extension's own rules. */
    path?: string;
    /** Effective leaf provenance available on ordinary settings pages. */
    sources?: FormattingSources<string>;
    /** Backend-authoritative layer chain for a template or repository output. */
    resolution?: SyncFileFormattingResolution;
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

  type GroupKey = (typeof FORMATTING_GROUPS)[number]['key'];

  const fileGroup = $derived(groupForPath(path));
  const relevantGroups = $derived(
    path === undefined
      ? FORMATTING_GROUPS
      : fileGroup === null
        ? []
        : FORMATTING_GROUPS.filter((group) => group.key === 'common' || group.key === fileGroup),
  );
  let activeGroup = $state<GroupKey>('common');
  const shownGroups = $derived(
    path === undefined
      ? relevantGroups.filter((group) => group.key === activeGroup)
      : relevantGroups,
  );
  const relevantFields = $derived(
    FORMATTING_FIELDS.filter((field) =>
      relevantGroups.some((group) => group.key === field.path[0]),
    ),
  );
  const dirtyKeySet = $derived(new Set(dirtyKeys));
  const valid = $derived(Object.keys(invalidNumbers).length === 0);
  const baseline = $derived(
    draft.preset === undefined ? inherited : FORMATTING_PRESETS[draft.preset],
  );
  const effective = $derived(applyFormattingPatch(inherited, draft));
  const presetField = FORMATTING_FIELDS[0];
  const overridden = $derived(
    relevantFields.filter((field) => formattingPatchValue(draft, field) !== undefined).length,
  );

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

  function groupForPath(filePath: string | undefined): GroupKey | null {
    if (filePath === undefined) return null;
    if (/\.json$/iu.test(filePath)) return 'json';
    if (/\.jsonc$/iu.test(filePath)) return 'jsonc';
    if (/\.ya?ml$/iu.test(filePath)) return 'yaml';
    if (/\.toml$/iu.test(filePath)) return 'toml';
    if (/\.(?:md|markdown)$/iu.test(filePath)) return 'markdown';
    return null;
  }

  function labelForSource(value: string): string {
    return SOURCE_LABEL[value] ?? value;
  }

  function sourceFor(field: FormattingField): string {
    const provenance = resolution?.provenance ?? sources;
    return provenance === undefined
      ? SOURCE_BY_SCOPE[scope]
      : labelForSource(formattingSourceValue(provenance, field));
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

{#if relevantGroups.length > 0}
  <div class="formatting-editor" data-valid={valid}>
    <section class="card group-card" aria-labelledby="formatting-{scope}-{idPrefix}-policy">
      <div class="group-head">
        <h3 class="group-name" id="formatting-{scope}-{idPrefix}-policy">Formatting</h3>
        <span class="group-tally"
          >{overridden} of {relevantFields.length} relevant rules overridden</span
        >
      </div>
      <p class="group-note">Presentation rules applied after semantic file merges</p>
      <div
        class={['policy-row', { 'is-unsaved': dirtyKeySet.has(presetField.key) }]}
        data-unsaved={dirtyKeySet.has(presetField.key) || undefined}
      >
        <span class="setting-say">
          <span class="setting-name"
            >{path === undefined ? 'Formatting preset' : 'File preset'}</span
          >
          <span class="setting-why">{presetField.description}</span>
        </span>
        <InheritControl
          label="Formatting preset"
          source={sourceFor(presetField)}
          inheritedValue={inherited.preset}
          inheritedLabel={optionLabel(inherited.preset)}
          value={draft.preset ?? null}
          options={presetField.options.map((value) => ({ value, label: optionLabel(value) }))}
          {disabled}
          onSelect={(value) => pick(presetField, value)}
          onRestore={() => clear(presetField)}
        />
      </div>
    </section>

    {#if resolution !== undefined}
      <section class="card layer-card" aria-label="Formatting precedence">
        <div class="group-head">
          <h3 class="group-name">Where formatting comes from</h3>
          <span class="group-tally">Later layers win</span>
        </div>
        <ol class="layer-rail">
          {#each resolution.layers as layer (layer.source)}
            <li class:is-current={layer.source === resolution.current_layer}>
              <span class="layer-dot" aria-hidden="true"></span>
              <span class="layer-name">{labelForSource(layer.source)}</span>
              <span class="layer-state">{optionLabel(layer.state)}</span>
              {#if layer.config_path !== undefined}
                <code>{layer.config_path}</code>
              {/if}
            </li>
          {/each}
        </ol>
      </section>
    {:else}
      <section class="card layer-card" aria-label="Formatting precedence">
        <div class="group-head">
          <h3 class="group-name">Where formatting comes from</h3>
          <span class="group-tally">Later layers win</span>
        </div>
        <ol class="layer-rail">
          {#each LAYERS_BY_SCOPE[scope] as layer, index (layer)}
            <li class:is-current={index === LAYERS_BY_SCOPE[scope].length - 1}>
              <span class="layer-dot" aria-hidden="true"></span>
              <span class="layer-name">{layer}</span>
              <span class="layer-state"
                >{index === LAYERS_BY_SCOPE[scope].length - 1 ? 'Editing here' : 'Inherited'}</span
              >
            </li>
          {/each}
        </ol>
      </section>
    {/if}

    {#if path === undefined}
      <nav class="group-tabs" aria-label="Formatting file type">
        <SegmentedControl
          name="formatting-group-{scope}-{idPrefix}"
          label="Formatting file type"
          options={relevantGroups.map((group) => ({ value: group.key, label: group.label }))}
          value={activeGroup}
          fluid
          onSelect={(value) => (activeGroup = value as GroupKey)}
        />
      </nav>
    {/if}

    {#each shownGroups as group (group.key)}
      <section class="card group-card" aria-labelledby="formatting-{scope}-{idPrefix}-{group.key}">
        <div class="group-head">
          <h3 class="group-name" id="formatting-{scope}-{idPrefix}-{group.key}">{group.label}</h3>
          <span class="group-tally"
            >{fieldsIn(group.key).filter(
              (field) => formattingPatchValue(draft, field) !== undefined,
            ).length} of {fieldsIn(group.key).length} overridden</span
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
                <InheritControl
                  label={fieldLabel(field)}
                  source={sourceFor(field)}
                  inheritedValue={String(formattingPolicyValue(baseline, field))}
                  inheritedLabel={optionLabel(String(formattingPolicyValue(baseline, field)))}
                  value={formattingPatchValue(draft, field)?.toString() ?? null}
                  options={field.options.map((value) => ({ value, label: optionLabel(value) }))}
                  {disabled}
                  onSelect={(value) => pick(field, value)}
                  onRestore={() => clear(field)}
                />
              {:else}
                <span class="number-control">
                  {#if formattingPatchValue(draft, field) !== undefined}
                    <AppTooltip text="Stop overriding - follow {sourceFor(field)}">
                      {#snippet children(attributes)}
                        <button
                          {...attributes}
                          type="button"
                          class="link-toggle broken"
                          aria-label="Stop overriding {fieldLabel(field)}"
                          {disabled}
                          onclick={() => clear(field)}
                        >
                          <Icon name="link-off" size={14} strokeWidth={2} />
                        </button>
                      {/snippet}
                    </AppTooltip>
                  {:else}
                    <AppTooltip
                      text="Follows {sourceFor(field)} · currently {formattingPolicyValue(
                        baseline,
                        field,
                      )}"
                    >
                      {#snippet children(attributes)}
                        <span {...attributes} class="link-toggle">
                          <Icon name="link" size={14} strokeWidth={2} />
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
                  <span class="number-source">From {sourceFor(field)}</span>
                </span>
              {/if}
            </div>
          {/each}
        </div>
      </section>
    {/each}

    <span class="effective-summary" aria-live="polite">
      Effective preset: {optionLabel(effective.preset)}
    </span>
  </div>
{/if}

<style>
  .formatting-editor {
    display: grid;
    gap: var(--space-4);
  }

  .card {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    padding: var(--space-5);
  }

  .group-tabs :global(fieldset) {
    max-width: 100%;
    width: 100%;
  }

  .layer-rail {
    display: flex;
    gap: 0;
    list-style: none;
    margin: var(--space-4) 0 0;
    overflow-x: auto;
    padding: 0;
  }

  .layer-rail li {
    align-items: center;
    color: var(--text-muted);
    display: grid;
    flex: 1 0 max-content;
    font-size: var(--font-size-micro);
    gap: var(--space-1);
    grid-template-columns: auto 1fr;
    min-width: 8rem;
    padding-inline-end: var(--space-4);
    position: relative;
  }

  .layer-rail li:not(:last-child)::after {
    background: var(--border-subtle);
    content: '';
    height: 1px;
    left: 10px;
    position: absolute;
    right: -10px;
    top: 5px;
    z-index: 0;
  }

  .layer-dot {
    background: var(--surface-base);
    border: 2px solid var(--border-control);
    border-radius: 999px;
    height: 10px;
    position: relative;
    width: 10px;
    z-index: 1;
  }

  .layer-rail li.is-current .layer-dot {
    background: var(--brand-action);
    border-color: var(--brand-action);
  }

  .layer-name {
    color: var(--text);
    font-weight: 600;
  }

  .layer-state,
  .layer-rail code {
    grid-column: 2;
  }

  .layer-rail code {
    color: var(--text-muted);
    font-size: inherit;
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
  }

  .group-note,
  .group-tally,
  .setting-why,
  .effective-summary {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
  }

  .group-note {
    margin: 0 0 var(--space-3);
  }

  .group-tally {
    white-space: nowrap;
  }

  .policy-rows {
    display: grid;
  }

  .policy-row {
    align-items: center;
    display: grid;
    gap: var(--space-3) var(--space-4);
    grid-template-columns: minmax(14rem, 1fr) auto;
    min-block-size: 48px;
    padding: var(--space-4) var(--space-2);
    position: relative;
  }

  .policy-row.is-unsaved {
    background: color-mix(in srgb, var(--brand-action-tint) 45%, transparent);
    box-shadow: inset 2px 0 var(--brand-action);
  }

  .policy-row:not(:last-child)::after {
    background: var(--border-subtle);
    block-size: 1px;
    bottom: 0;
    content: '';
    inset-inline: var(--space-2);
    position: absolute;
  }

  .setting-say {
    display: grid;
    gap: var(--space-3);
    min-width: 0;
  }

  .setting-name {
    font-size: var(--font-size-meta);
    font-weight: 600;
    min-block-size: 10px;
    text-box: trim-both cap alphabetic;
  }

  .setting-why {
    min-block-size: 9px;
    text-box: trim-both cap alphabetic;
  }

  .number-control {
    align-items: center;
    display: grid;
    gap: var(--space-2);
    grid-template-columns: var(--inherit-marker-size) 6rem;
    justify-self: end;
  }

  .number-source {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
    grid-column: 2;
    text-align: end;
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

  @media (max-width: 900px) {
    .policy-row {
      align-items: start;
      grid-template-columns: 1fr;
    }

    .number-control {
      justify-self: start;
    }
  }

  @media (max-width: 30rem) {
    .card {
      padding: var(--space-3);
    }

    .group-head {
      flex-wrap: wrap;
    }

    .policy-row {
      padding-inline: 0;
    }
  }
</style>

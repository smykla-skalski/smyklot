<script lang="ts">
  type SegmentTone = 'default' | 'accent' | 'on' | 'off';

  interface SegmentOption {
    value: string;
    label: string;
    tone?: SegmentTone;
    badge?: string | number;
  }

  const {
    name,
    label,
    descriptionId,
    options,
    value,
    disabled = false,
    align = 'start',
    compact = false,
    variant = 'default',
    onSelect,
  }: {
    name: string;
    label: string;
    descriptionId?: string;
    options: ReadonlyArray<SegmentOption>;
    value: string;
    disabled?: boolean;
    align?: 'start' | 'end';
    compact?: boolean;
    variant?: 'default' | 'navigation';
    onSelect: (value: string) => void;
  } = $props();

  const selectedTone = $derived(
    options.find((option) => option.value === value)?.tone ?? 'default',
  );

  function positionSelection(node: HTMLFieldSetElement, selection: string) {
    let frame: number | undefined;
    let currentSelection = selection;

    function scheduleMove(): void {
      if (frame !== undefined) cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        frame = requestAnimationFrame(() => {
          frame = undefined;
          const option = node.querySelector<HTMLInputElement>('input:checked')?.closest('label');
          if (option === null || option === undefined) return;
          node.style.setProperty('--segment-left', `${option.offsetLeft}px`);
          node.style.setProperty('--segment-width', `${option.offsetWidth}px`);
          node.classList.add('selection-ready');
        });
      });
    }

    scheduleMove();

    return {
      update(nextSelection: string) {
        if (nextSelection === currentSelection) return;
        currentSelection = nextSelection;
        scheduleMove();
      },
      destroy() {
        if (frame !== undefined) cancelAnimationFrame(frame);
      },
    };
  }
</script>

<fieldset
  class={[
    align === 'end' && 'align-end',
    compact && 'compact',
    variant === 'navigation' && 'navigation',
  ]}
  class:selected-accent={selectedTone === 'accent'}
  class:selected-on={selectedTone === 'on'}
  class:selected-off={selectedTone === 'off'}
  aria-describedby={descriptionId}
  use:positionSelection={value}
  {disabled}
>
  <legend>{label}</legend>
  <span class="selection-indicator" aria-hidden="true"></span>
  {#each options as option (option.value)}
    <label>
      <input
        type="radio"
        {name}
        value={option.value}
        checked={value === option.value}
        onchange={(event) => onSelect(event.currentTarget.value)}
      />
      <span class="segment-label">
        <span>{option.label}</span>
        {#if option.badge !== undefined}
          <sup class="segment-badge"><span>{option.badge}</span></sup>
        {/if}
      </span>
    </label>
  {/each}
</fieldset>

<style>
  fieldset {
    --selected-bg: var(--surface-base);
    --selected-stroke: color-mix(in srgb, var(--text-secondary) 20%, var(--surface-base));
    --selected-text: var(--text-secondary);
    background: var(--well);
    border: 1px solid var(--rule);
    border-radius: var(--r-ctl);
    display: inline-flex;
    flex: none;
    height: var(--control-height);
    isolation: isolate;
    margin: 0;
    min-width: 0;
    overflow: clip;
    padding: var(--control-inset);
    position: relative;
  }

  fieldset.selected-accent {
    --selected-bg: var(--brand-action-tint);
    --selected-stroke: color-mix(in srgb, var(--brand-action-text) 20%, var(--brand-action-tint));
    --selected-text: var(--brand-action-text);
  }

  fieldset.selected-on {
    --selected-bg: var(--success-tint);
    --selected-stroke: color-mix(in srgb, var(--success) 20%, var(--success-tint));
    --selected-text: var(--success);
  }

  fieldset.selected-off {
    --selected-bg: var(--danger-tint);
    --selected-stroke: color-mix(in srgb, var(--danger) 20%, var(--danger-tint));
    --selected-text: var(--danger);
  }

  fieldset.align-end {
    justify-self: end;
  }

  fieldset.compact {
    height: var(--control-height-compact);
  }

  fieldset.compact .segment-label {
    font-size: var(--font-size-micro);
    min-width: 2.25rem;
    padding: 0 8px;
  }

  fieldset.navigation .segment-label {
    font-size: var(--font-size-body);
    gap: var(--space-2);
    min-width: 0;
    padding: 0 var(--space-3);
  }

  legend {
    clip-path: inset(50%);
    height: 1px;
    overflow: hidden;
    position: absolute;
    white-space: nowrap;
    width: 1px;
  }

  label {
    cursor: pointer;
    display: flex;
    height: 100%;
    position: relative;
  }

  label::before {
    background: var(--strip-lift);
    border-radius: calc(var(--r-ctl) - 3px);
    content: '';
    inset: 0;
    opacity: 0;
    pointer-events: none;
    position: absolute;
    transition: opacity 120ms ease-out;
    z-index: 1;
  }

  label:hover:not(:has(input:disabled))::before {
    opacity: 1;
  }

  label:has(input:checked) + label:hover::before {
    border-radius: 0 calc(var(--r-ctl) - 3px) calc(var(--r-ctl) - 3px) 0;
    inset-inline-start: calc(-1 * var(--r-ctl));
  }

  label:hover:has(+ label input:checked)::before {
    border-radius: calc(var(--r-ctl) - 3px) 0 0 calc(var(--r-ctl) - 3px);
    inset-inline-end: calc(-1 * var(--r-ctl));
  }

  input {
    height: 1px;
    opacity: 0;
    position: absolute;
    width: 1px;
  }

  .segment-label {
    align-items: center;
    border-radius: calc(var(--r-ctl) - 3px);
    color: var(--text-secondary);
    display: flex;
    font-size: 0.6875rem;
    font-weight: 600;
    height: 100%;
    justify-content: center;
    line-height: 1;
    padding: 0 0.5rem;
    position: relative;
    transition:
      color 180ms ease-out,
      transform var(--duration-press) var(--ease-standard);
    z-index: 3;
  }

  .segment-badge {
    align-items: center;
    background: var(--surface-raised);
    border: 1px solid var(--border-subtle);
    border-radius: 0.25rem;
    color: var(--text-muted);
    display: inline-grid;
    font: 700 0.5625rem / 1 var(--mono);
    font-variant-numeric: tabular-nums;
    height: 1rem;
    justify-content: center;
    min-width: 1.125rem;
    padding: 0 0.25rem;
    place-items: center;
    position: relative;
    top: -0.38rem;
    vertical-align: super;
  }

  .segment-badge > span {
    display: grid;
    height: 100%;
    line-height: 1;
    place-items: center;
    width: 100%;
  }

  input:checked ~ .segment-label .segment-badge {
    background: var(--brand-action-tint);
    border-color: color-mix(in srgb, var(--brand-action-text) 20%, var(--brand-action-tint));
    color: var(--brand-action-text);
  }

  input:checked ~ .segment-label {
    color: var(--selected-text);
  }

  label:hover input:not(:checked):not(:disabled) ~ .segment-label {
    color: var(--text);
  }

  label:active input:not(:disabled) ~ .segment-label {
    transform: scale(0.97);
  }

  .selection-indicator {
    background: var(--selected-bg);
    border-radius: calc(var(--r-ctl) - 3px);
    box-shadow: inset 0 0 0 1px var(--selected-stroke);
    bottom: var(--control-inset);
    left: var(--segment-left, var(--control-inset));
    pointer-events: none;
    position: absolute;
    top: var(--control-inset);
    transition:
      left 240ms cubic-bezier(0.22, 1, 0.36, 1),
      width 240ms cubic-bezier(0.22, 1, 0.36, 1),
      background-color var(--duration-fast) var(--ease-standard),
      box-shadow var(--duration-fast) var(--ease-standard);
    width: var(--segment-width, 0);
    z-index: 2;
  }

  fieldset:not(.selection-ready) .selection-indicator {
    transition: none;
  }

  fieldset:has(label:hover input:checked:not(:disabled)) .selection-indicator {
    background: color-mix(in srgb, var(--selected-text) 8%, var(--selected-bg));
  }

  fieldset:has(label:active input:checked:not(:disabled)) .selection-indicator {
    background: color-mix(in srgb, var(--selected-text) 16%, var(--selected-bg));
  }

  input:focus-visible ~ .segment-label {
    outline: 2px solid var(--brand);
    outline-offset: -2px;
  }

  input:disabled ~ .segment-label,
  fieldset:disabled .selection-indicator {
    opacity: 0.45;
  }

  fieldset:disabled label {
    cursor: default;
  }

  @media (max-width: 36rem) {
    fieldset.navigation .segment-label {
      padding-inline: var(--space-2);
    }
  }
</style>

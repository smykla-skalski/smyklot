<script lang="ts">
  type SegmentTone = 'default' | 'accent' | 'on' | 'off';

  interface SegmentOption {
    value: string;
    label: string;
    tone?: SegmentTone;
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
    onSelect: (value: string) => void;
  } = $props();

  function animateSelection(node: HTMLFieldSetElement, selection: string) {
    let activeAnimation: Animation | undefined;
    let activeFill: HTMLElement | undefined;
    let frame: number | undefined;
    let currentSelection = selection;

    function selectedFill(): HTMLElement | null {
      const option = node.querySelector<HTMLInputElement>('input:checked')?.closest('label');
      return option?.querySelector<HTMLElement>('.segment-fill') ?? null;
    }

    function moveSelection(animate: boolean): void {
      const nextFill = selectedFill();
      if (nextFill === null) return;

      const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
      if (!animate || activeFill === undefined || reduceMotion) {
        activeAnimation?.cancel();
        activeAnimation = undefined;
        activeFill = nextFill;
        return;
      }

      const currentRect = activeFill.getBoundingClientRect();
      const currentColor = getComputedStyle(activeFill).backgroundColor;
      activeAnimation?.cancel();

      const targetRect = nextFill.getBoundingClientRect();
      const targetColor = getComputedStyle(nextFill).backgroundColor;
      const translateX = currentRect.left - targetRect.left;
      const scaleX = currentRect.width / targetRect.width;
      const animation = nextFill.animate(
        [
          {
            backgroundColor: currentColor,
            transform: `translate3d(${translateX}px, 0, 0) scaleX(${scaleX})`,
          },
          {
            backgroundColor: targetColor,
            transform: 'translate3d(0, 0, 0) scaleX(1)',
          },
        ],
        {
          duration: 240,
          easing: 'cubic-bezier(0.22, 1, 0.36, 1)',
        },
      );
      activeAnimation = animation;
      activeFill = nextFill;
      animation.onfinish = () => {
        if (activeAnimation === animation) activeAnimation = undefined;
      };
    }

    function scheduleMove(animate: boolean): void {
      if (frame !== undefined) cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        frame = undefined;
        moveSelection(animate);
      });
    }

    scheduleMove(false);

    return {
      update(nextSelection: string) {
        if (nextSelection === currentSelection) return;
        currentSelection = nextSelection;
        scheduleMove(true);
      },
      destroy() {
        if (frame !== undefined) cancelAnimationFrame(frame);
        activeAnimation?.cancel();
      },
    };
  }
</script>

<fieldset
  class={[align === 'end' && 'align-end', compact && 'compact']}
  aria-describedby={descriptionId}
  use:animateSelection={value}
  {disabled}
>
  <legend>{label}</legend>
  {#each options as option (option.value)}
    <label
      class:segment-accent={option.tone === 'accent'}
      class:segment-on={option.tone === 'on'}
      class:segment-off={option.tone === 'off'}
    >
      <input
        type="radio"
        {name}
        value={option.value}
        checked={value === option.value}
        onchange={(event) => onSelect(event.currentTarget.value)}
      />
      <span class="segment-fill" aria-hidden="true"></span>
      <span class="segment-label">{option.label}</span>
    </label>
  {/each}
</fieldset>

<style>
  fieldset {
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
    color: var(--dim);
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

  input:checked ~ .segment-label {
    color: var(--signal);
  }

  .segment-accent input:checked ~ .segment-label {
    color: var(--brand-action-text);
  }

  .segment-on input:checked ~ .segment-label {
    color: var(--clear);
  }

  .segment-off input:checked ~ .segment-label {
    color: var(--stop);
  }

  label:hover input:not(:checked):not(:disabled) ~ .segment-label {
    color: var(--text);
  }

  label:active input:not(:disabled) ~ .segment-label {
    transform: scale(0.97);
  }

  .segment-fill {
    background: var(--signal-tint);
    border-radius: calc(var(--r-ctl) - 3px);
    inset: 0;
    opacity: 0;
    pointer-events: none;
    position: absolute;
    transform-origin: left center;
    will-change: transform;
    z-index: 2;
  }

  .segment-accent .segment-fill {
    background: var(--brand-action-tint);
  }

  .segment-on .segment-fill {
    background: var(--clear-tint);
  }

  .segment-off .segment-fill {
    background: var(--stop-tint);
  }

  input:checked ~ .segment-fill {
    opacity: 1;
  }

  input:focus-visible ~ .segment-label {
    outline: 2px solid var(--brand);
    outline-offset: -2px;
  }

  input:disabled ~ .segment-label,
  input:checked:disabled ~ .segment-fill {
    opacity: 0.45;
  }

  fieldset:disabled label {
    cursor: default;
  }
</style>

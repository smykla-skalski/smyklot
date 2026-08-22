<script lang="ts">
  const {
    checked,
    label,
    word,
    bare = false,
    disabled = false,
    onToggle,
  }: {
    checked: boolean;
    /** The accessible name; required because the track alone says nothing. */
    label: string;
    /** An optional visible word beside the track ("Syncing"). */
    word?: string;
    /**
     * Drops the 44px tap box so the switch cannot inflate the line it sits
     * on - for a head or a card row that sets its own rhythm. The hit area
     * survives on the input itself.
     */
    bare?: boolean;
    disabled?: boolean;
    onToggle: (next: boolean) => void;
  } = $props();
</script>

<!-- A setting that has already taken effect: flipping it IS the change, so
     the control is an input rather than a button that opens a form. -->
<label class="switch" class:bare>
  {#if word !== undefined}<span class="switch-word">{word}</span>{/if}
  <input
    type="checkbox"
    {checked}
    {disabled}
    aria-label={label}
    onchange={(event) => onToggle(event.currentTarget.checked)}
  />
  <span class="switch-track"></span>
</label>

<style>
  .switch {
    align-items: center;
    display: inline-flex;
    gap: var(--space-2);
    min-block-size: var(--touch-target, 2.75rem);
  }

  /* Where the switch stands beside content that sets the rhythm (a card
     head), the tap box must not inflate the row - the hit area survives on
     the input itself. */
  .switch.bare {
    min-block-size: auto;
  }

  .switch input {
    position: absolute;
    opacity: 0;
    pointer-events: none;
  }

  .switch-track {
    background: var(--switch-track-off);
    border: 1px solid var(--switch-track-off-border);
    border-radius: 999px;
    block-size: 20px;
    cursor: pointer;
    inline-size: 34px;
    padding: 1px;
    position: relative;
    transition:
      background var(--duration-fast) var(--ease-standard),
      border-color var(--duration-fast) var(--ease-standard);
  }

  .switch-track::after {
    background: var(--switch-thumb);
    border-radius: 50%;
    block-size: 16px;
    box-shadow: 0 1px 2px var(--shadow-color);
    content: '';
    inset-block-start: 1px;
    inset-inline-start: 1px;
    position: absolute;
    inline-size: 16px;
    transition: translate var(--duration-fast) var(--ease-standard);
  }

  .switch input:checked + .switch-track {
    background: var(--switch-track-on);
    border-color: var(--switch-track-on);
  }

  .switch input:checked + .switch-track::after {
    translate: 14px 0;
  }

  .switch input:focus-visible + .switch-track {
    outline: 2px solid var(--focus);
    outline-offset: 2px;
  }

  .switch input:disabled + .switch-track {
    cursor: not-allowed;
    opacity: 0.5;
  }

  .switch-word {
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
    text-box: trim-both cap alphabetic;
  }
</style>

<script lang="ts">
  /**
   * An instant boolean: flipping it is the act.
   *
   * The contract that separates it from every other control on the page - a
   * switch's effect has already landed when the thumb comes to rest. Nothing
   * to save, nothing pending. Where a value waits for a Save, that is a form
   * control; where choosing needs a sentence, that is a radio card. The old
   * sync page said all of these with one segmented control, which is the
   * ambiguity this component retires.
   *
   * Controlled, like `SegmentedControl`: the caller owns the value and hears
   * about the wish through `onChange`. A save that fails puts the old value
   * back by re-rendering, and the thumb follows.
   */
  const {
    checked,
    onChange,
    label,
    ariaLabel,
    disabled = false,
    describedBy,
  }: {
    checked: boolean;
    onChange: (next: boolean) => void;
    /** The word beside the track. Omitted where the row already names it. */
    label?: string;
    /**
     * The name for a switch with no visible word - a row already carries it in
     * ink, and a switch with neither is a control that announces itself as
     * "checkbox" and nothing else.
     */
    ariaLabel?: string;
    disabled?: boolean;
    describedBy?: string;
  } = $props();
</script>

<label class="switch">
  {#if label !== undefined}
    <span class="switch-word cap-trim">{label}</span>
  {/if}
  <input
    type="checkbox"
    role="switch"
    {checked}
    {disabled}
    aria-label={label === undefined ? ariaLabel : undefined}
    aria-describedby={describedBy}
    onchange={(event) => {
      const wanted = event.currentTarget.checked;
      /* Put the box back where the value says, and let `checked` move it.
         A checkbox flips ITSELF on a click, and Svelte re-applies an attribute
         only when the value it holds has changed - so a save that is refused
         leaves the prop exactly as it was, nothing is re-applied, and the
         browser's own flip stays on screen saying the opposite of what is
         stored. That is what makes this controlled rather than only described
         as controlled: the accepted case changes `checked` and moves the thumb
         a beat later, and the refused case has already been undone here. */
      event.currentTarget.checked = checked;
      onChange(wanted);
    }}
  />
  <span class="switch-track" aria-hidden="true"></span>
</label>

<style>
  .switch {
    align-items: center;
    display: inline-flex;
    gap: 0.6rem;
  }

  .switch-word {
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
  }

  /* The input stays in the tree for focus and the accessibility name; the
     track draws the state. */
  input {
    block-size: 1px;
    inline-size: 1px;
    margin: 0;
    opacity: 0;
    position: absolute;
  }

  .switch-track {
    background: var(--switch-track-off);
    border: 1px solid var(--switch-track-off-border);
    border-radius: 999px;
    block-size: 20px;
    cursor: pointer;
    inline-size: 34px;
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
    inline-size: 16px;
    inset-block-start: 1px;
    inset-inline-start: 1px;
    position: absolute;
    transition: translate var(--duration-fast) var(--ease-standard);
  }

  input:checked + .switch-track {
    background: var(--switch-track-on);
    border-color: var(--switch-track-on);
  }

  input:checked + .switch-track::after {
    translate: 14px 0;
  }

  input:focus-visible + .switch-track {
    outline: 2px solid var(--focus);
    outline-offset: 2px;
  }

  input:disabled + .switch-track {
    cursor: not-allowed;
    opacity: 0.5;
  }

  @media (prefers-reduced-motion: reduce) {
    .switch-track,
    .switch-track::after {
      transition: none;
    }
  }
</style>

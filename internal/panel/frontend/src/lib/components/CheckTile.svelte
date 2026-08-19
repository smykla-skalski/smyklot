<script lang="ts">
  /**
   * A checkbox drawn as a tile, with the value it names inside it.
   *
   * The native input is still the control - it is what a keyboard reaches, what a
   * screen reader announces and what carries `:checked`, `:focus-visible` and
   * `:disabled`. It is visually hidden rather than replaced, and every state below is
   * drawn from it with a sibling selector. Replacing it with a div and an `onclick`
   * would mean rebuilding all four states by hand and getting one of them wrong.
   *
   * Ninety lines of CSS served one `<label>` in `ConfigEditor`, which is the shape of
   * a component that has not been written yet: the anatomy is fixed, the states are
   * intricate, and nothing else could use it without copying all of it.
   */
  const {
    label,
    checked = false,
    disabled = false,
    onchange,
  }: {
    /** The value this tile names. Drawn as code, because these are command words. */
    label: string;
    checked?: boolean;
    /**
     * Off, and stays off.
     *
     * A tile is disabled for two different reasons in the editor - the whole editor is
     * read-only, or this is the last one still checked and unchecking it would leave
     * none. Both are the caller's to decide; the tile only draws the answer.
     */
    disabled?: boolean;
    onchange?: () => void;
  } = $props();
</script>

<label class="check-tile">
  <input type="checkbox" {checked} {disabled} onchange={() => onchange?.()} />
  <span class="check-box" aria-hidden="true">
    <svg viewBox="0 0 12 12"><path d="M2.2 6.4 4.9 9 9.8 3.2" /></svg>
  </span>
  <code class="band-trim">{label}</code>
</label>

<style>
  .check-tile {
    align-items: center;
    background: var(--strip-lift);
    border: 1px solid var(--rule);
    border-radius: var(--r-ctl);
    cursor: pointer;
    display: inline-flex;
    gap: 0.5625rem;
    min-height: 2.25rem;
    padding: 0 0.8125rem 0 0.625rem;
    transition:
      background-color 120ms ease-out,
      border-color 120ms ease-out;
  }

  .check-tile:hover:not(:has(input:disabled)) {
    border-color: var(--border-strong);
  }

  .check-tile:has(input:disabled) {
    cursor: default;
  }

  .check-tile input {
    height: 1px;
    opacity: 0;
    position: absolute;
    width: 1px;
  }

  .check-box {
    background: var(--strip);
    border: 1.5px solid var(--border-strong);
    border-radius: 5px;
    flex: none;
    height: 1.125rem;
    position: relative;
    transition:
      background-color 130ms ease-out,
      border-color 130ms ease-out;
    width: 1.125rem;
  }

  .check-box svg {
    fill: none;
    height: 12px;
    inset: 0;
    margin: auto;
    position: absolute;
    stroke: var(--on-admin);
    stroke-dasharray: 14;
    stroke-dashoffset: 14;
    stroke-linecap: round;
    stroke-linejoin: round;
    stroke-width: 2.4;
    transition: stroke-dashoffset 160ms var(--ease-standard) 40ms;
    width: 12px;
  }

  .check-tile input:checked + .check-box {
    background: var(--admin);
    border-color: var(--admin);
  }

  .check-tile input:checked + .check-box svg {
    stroke-dashoffset: 0;
  }

  .check-tile input:focus-visible + .check-box {
    outline: 2px solid var(--brand);
    outline-offset: 2px;
  }

  .check-tile input:disabled + .check-box {
    opacity: 0.7;
  }

  .check-tile code {
    background: transparent;
    color: var(--dim);
    font-size: var(--font-size-control);
    padding: 0;
    transition: color 120ms ease-out;
  }

  .check-tile input:checked ~ code {
    color: var(--text);
    font-weight: 500;
  }

  @media (prefers-reduced-motion: reduce) {
    .check-box svg {
      transition: none;
    }
  }
</style>

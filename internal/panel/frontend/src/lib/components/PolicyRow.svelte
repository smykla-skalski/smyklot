<script lang="ts">
  /**
   * One thing this installation decides, and what it decided.
   *
   * The value is said twice: once as a word, once as the control holding it.
   * A column of switches is read by decoding thumb positions, which is slower
   * than reading `On`, `Off`, `Squash` down the same column - and the word is
   * what a screen reader and a screenshot both carry.
   *
   * The clear at the end removes the management. It never writes a value: the
   * opposite of "managed and off" is "not managed", and a setting that stops
   * being managed goes back to whatever each repository says, which is a
   * different sentence from "off everywhere".
   */
  import Icon from '#lib/components/Icon.svelte';

  const {
    name,
    why,
    value,
    control,
    params,
    open,
    isOpen = false,
    clearLabel,
    onStopManaging,
  }: {
    name: string;
    /** What it means, in the reader's words rather than the API's. */
    why?: string;
    /** The value as a word - `On`, `Off`, `Squash`. Omit where a word cannot say it. */
    value?: string;
    /** The control itself: a switch, a select, a chip row. */
    control: import('svelte').Snippet;
    /**
     * What the thing is set to, where a word cannot hold it: a rule's own
     * parameters, worn as quiet chips under its name. They belong to the name
     * rather than to the control, because they are what the row is saying.
     */
    params?: import('svelte').Snippet;
    /** The row opened for editing, in place and full width. */
    open?: import('svelte').Snippet;
    /**
     * Whether that editor is showing. A prop rather than "was a snippet given",
     * because one snippet is passed for a whole list of rows and only one of
     * them is open - and an empty editor still draws its own rule and padding.
     */
    isOpen?: boolean;
    /** What the clear does here, where it is not "stop managing" - "Switch the rule off". */
    clearLabel?: string;
    /** Offered where the setting can stop being managed. */
    onStopManaging?: () => void;
  } = $props();
</script>

<div class="policy-row">
  <span class="setting-say band-trim-stack">
    <span class="setting-name">{name}</span>
    {#if why !== undefined}
      <span class="setting-why">{why}</span>
    {/if}
    {#if params !== undefined}
      <span class="rule-params">{@render params()}</span>
    {/if}
  </span>
  <span class="policy-value">
    {#if value !== undefined}
      <span class="value-word band-trim" class:is-on={value !== 'Off'}>{value}</span>
    {/if}
    {@render control()}
  </span>
  {#if onStopManaging !== undefined}
    <button
      type="button"
      class="setting-clear"
      title={clearLabel ?? 'Stop managing - repositories keep their own value'}
      aria-label="{clearLabel ?? 'Stop managing'} {name}"
      onclick={onStopManaging}
    >
      <Icon name="close" size={11} />
    </button>
  {/if}
  {#if isOpen && open !== undefined}
    <div class="rule-edit">{@render open()}</div>
  {/if}
</div>

<style>
  .policy-row {
    align-items: center;
    display: grid;
    gap: var(--space-2) var(--space-4);
    grid-template-columns: 1fr auto auto;
    min-block-size: 3.1rem;
    padding: 0.55rem var(--space-2);
  }

  .setting-say {
    display: grid;
    gap: 0.15rem;
  }

  /* Trimmed by `.band-trim-stack` on the container rather than here: the name
     was trimmed and the description under it was not, so the stack kept the
     why's leading below its baseline and the whole column's band sat 2.5px
     above the value beside it - on every policy row of all four sync forms. */
  .setting-name {
    font-size: var(--font-size-meta);
    font-weight: 500;
  }

  .setting-why {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    max-width: 52ch;
  }

  /* The parameters as quiet chips: what the rule is set to, said without
     opening it. Anything that needs a control to change is behind the row's own
     Edit; this is the reading of it. */
  .rule-params {
    display: flex;
    flex-wrap: wrap;
    gap: 0.3rem;
    margin-block-start: 0.15rem;
  }

  /* Full width and under everything, so the row it belongs to stays a row: an
     editor that pushed the name aside would move every row beneath it twice. */
  .rule-edit {
    border-top: 1px dashed var(--border-subtle);
    display: grid;
    gap: var(--space-3);
    grid-column: 1 / -1;
    margin-block-start: 0.55rem;
    padding-block-start: var(--space-3);
  }

  .policy-value {
    align-items: center;
    display: flex;
    gap: var(--space-3);
    justify-self: end;
  }

  /* The value in a word, so a column of these is read rather than decoded. */
  .value-word {
    color: var(--text-muted);
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    min-inline-size: 1.9rem;
    text-align: end;
  }

  .value-word.is-on {
    color: var(--text-secondary);
    font-weight: 600;
  }

  /* Quiet until the row is under the hand or under focus: it is available on
     every row and wanted on almost none of them. */
  .setting-clear {
    align-items: center;
    appearance: none;
    background: none;
    block-size: 26px;
    border: 0;
    border-radius: var(--r-ctl);
    color: var(--text-muted);
    cursor: pointer;
    display: inline-flex;
    inline-size: 26px;
    justify-content: center;
    opacity: 0.45;
    padding: 0;
    transition: opacity var(--duration-fast) var(--ease-standard);
  }

  .policy-row:hover .setting-clear,
  .policy-row:focus-within .setting-clear {
    opacity: 1;
  }

  .setting-clear:hover {
    background-image: linear-gradient(
      var(--interactive-hover-layer),
      var(--interactive-hover-layer)
    );
    color: var(--text-primary);
  }

  @media (max-width: 40rem) {
    .policy-row {
      grid-template-columns: 1fr auto;
    }

    .policy-value {
      grid-column: 1 / -1;
      justify-self: start;
    }
  }
</style>

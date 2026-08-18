<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLSelectAttributes } from 'svelte/elements';

  import Icon from './Icon.svelte';

  /**
   * A select, and the chevron that says it is one.
   *
   * A native `<select>` cannot be given an indicator of its own, so the panel wraps
   * it and draws one alongside - which is why the wrapper, not the select, is the
   * layout box. Nine call sites wrote that wrapper, that chevron and its exact size
   * and stroke by hand.
   *
   * Options come either as data or as markup: most call sites have a list, three
   * build their `<option>`s in a loop or want them keyed, and forcing either into
   * the other shape would be worse than accepting both.
   */
  let {
    value = $bindable(),
    options,
    class: extra = '',
    children,
    ...rest
  }: {
    value?: string | number;
    /** The straightforward case: a fixed list of values and their words. */
    options?: ReadonlyArray<{ value: string | number; label: string }>;
    /** The select's own class - `mono` for a command, say. Never the wrapper's layout. */
    class?: string;
    /** `<option>` elements, when they are built rather than listed. */
    children?: Snippet;
  } & HTMLSelectAttributes = $props();
</script>

<span class="select-wrap">
  <select bind:value class="select-input {extra}" {...rest}>
    {#if options !== undefined}
      {#each options as option (option.value)}
        <option value={option.value}>{option.label}</option>
      {/each}
    {:else if children !== undefined}
      {@render children()}
    {/if}
  </select>
  <Icon name="chevron-down" size={14} strokeWidth={2} />
</span>

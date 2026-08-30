<script lang="ts">
  import Select from './Select.svelte';

  const PAGE_SIZES = [10, 20, 50] as const;

  const {
    value,
    label,
    onSelect,
  }: {
    value: number;
    label: string;
    onSelect: (value: number) => void;
  } = $props();

  function select(event: Event): void {
    const nextValue = Number((event.currentTarget as HTMLSelectElement).value);
    if (PAGE_SIZES.some((size) => size === nextValue)) onSelect(nextValue);
  }
</script>

<!--
@component
How many rows a page shows, offered as 10, 20 or 50 and nothing else. The list is
closed on purpose: a size a reader can type is a number nobody has checked against the
column widths, and three steps is enough to say "a screenful", "a few screens" or "as
much as will load".

It guards its own answer rather than trusting the event - a `<select>` can be given a
value no option carried, so a size outside the three is dropped rather than passed on.

Belongs to a collection that has been counted. A table that loads on a cursor has no
total and no pages, so it has no size to choose either.
-->

<Select class="page-size" {value} aria-label={label} onchange={select}>
  {#each PAGE_SIZES as size (size)}
    <option value={size}>{size}</option>
  {/each}
</Select>

<style>
  /* The wrapper carries the width now that it, not the select, is the layout
     box its row sees. `:global` because both elements are `Select`'s. */
  :global(.select-wrap) {
    width: 4rem;
  }

  :global(.page-size) {
    font-size: var(--font-size-meta);
    height: var(--local-control-height, var(--control-height));
    min-width: 4rem;
  }
</style>

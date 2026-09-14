<script lang="ts">
  import Select from './Select.svelte';

  const PAGE_SIZES = [10, 20, 50] as const;

  const {
    value,
    label,
    onSelect,
    disabled = false,
  }: {
    disabled?: boolean;
    value: number;
    label: string;
    onSelect: (value: number) => void;
  } = $props();

  function select(nextValue: number): void {
    if (PAGE_SIZES.some((size) => size === nextValue)) onSelect(nextValue);
  }
</script>

<!--
@component
How many rows a page shows, offered as 10, 20 or 50 and nothing else. The list is
closed on purpose: a size a reader can type is a number nobody has checked against the
column widths, and three steps is enough to say "a screenful", "a few screens" or "as
much as will load".

Its numeric answer is checked against the supported page sizes before it leaves the component.

Used by numbered and cursor pagination. Changing the size resets the current
pagination boundary so pages cannot overlap or skip rows.
-->

<span class="page-size">
  <Select
    {value}
    {disabled}
    aria-label={label}
    onValueChange={select}
    options={PAGE_SIZES.map((size) => ({ value: size, label: String(size) }))}
  />
</span>

<style>
  .page-size {
    display: inline-block;
    inline-size: 4rem;
  }
</style>

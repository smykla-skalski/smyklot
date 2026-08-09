<script lang="ts">
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

<select class="select-input page-size" {value} aria-label={label} onchange={select}>
  {#each PAGE_SIZES as size (size)}
    <option value={size}>{size}</option>
  {/each}
</select>

<style>
  .page-size {
    font-size: var(--font-size-meta);
    height: var(--local-control-height, var(--control-height));
    min-width: 4rem;
    width: 4rem;
  }
</style>

<script lang="ts">
  import Icon from './Icon.svelte';

  const {
    label,
    placeholder,
    value,
    onInput,
  }: {
    label: string;
    placeholder: string;
    value: string;
    onInput: (value: string) => void;
  } = $props();
</script>

<label class="search-field">
  <span class="visually-hidden">{label}</span>
  <span class="search-icon"><Icon name="search" size={14} strokeWidth={2} /></span>
  <input
    class="text-input"
    type="search"
    {placeholder}
    {value}
    oninput={(event) => onInput(event.currentTarget.value)}
  />
</label>

<style>
  .search-field {
    align-items: center;
    display: flex;
    /* Fills the room it is given, which is what seven of the eight toolbars
       holding one want. The sync toolbar sets both, because the mock draws its
       search at a fixed width against a filter on the far side of the row. */
    flex: var(--search-field-flex, 1 1 15rem);
    min-width: 0;
    position: relative;
    width: var(--search-field-width, 100%);
  }

  input {
    font-size: var(--font-size-meta);
    height: var(--local-control-height, var(--control-height-compact));
    padding-left: 2.1875rem;
    width: 100%;
  }

  .search-icon {
    color: var(--text-muted);
    display: grid;
    left: 0.8125rem;
    place-items: center;
    pointer-events: none;
    position: absolute;
    top: 50%;
    transform: translateY(-50%);
  }
</style>

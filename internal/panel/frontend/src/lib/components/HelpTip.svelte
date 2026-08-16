<script lang="ts">
  import { tooltip } from '../tooltip';
  import Icon from './Icon.svelte';

  const {
    id,
    label,
    text,
    align = 'end',
  }: {
    id: string;
    label: string;
    text: string;
    align?: 'start' | 'end';
  } = $props();
</script>

<span class="help-tip" class:align-start={align === 'start'}>
  <button use:tooltip={{ id, text, align }} type="button" aria-label={label} aria-describedby={id}>
    <Icon name="info" size={14} strokeWidth={2} />
  </button>
</span>

<style>
  .help-tip {
    display: inline-flex;
  }

  button {
    background: transparent;
    border: 0;
    border-radius: var(--r-ctl);
    color: var(--dim);
    cursor: help;
    display: inline-grid;
    height: 1.125rem;
    padding: 0;
    place-items: center;
    width: 1.125rem;
  }

  button:hover,
  button:focus-visible {
    color: var(--signal);
  }
</style>

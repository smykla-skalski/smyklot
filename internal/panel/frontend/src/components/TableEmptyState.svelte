<script lang="ts">
  import Icon from './Icon.svelte';

  const {
    title,
    description,
    actionLabel,
    onAction,
  }: {
    title: string;
    description: string;
    actionLabel?: string;
    onAction?: () => void;
  } = $props();
</script>

<div class="table-empty-state">
  <span class="empty-symbol" aria-hidden="true"><Icon name="search" size={22} /></span>
  <strong>{title}</strong>
  <span>{description}</span>
  {#if actionLabel !== undefined && onAction !== undefined}
    <button class="btn" type="button" onclick={onAction}>{actionLabel}</button>
  {/if}
</div>

<style>
  .table-empty-state {
    align-items: center;
    color: var(--text-muted);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    justify-content: center;
    max-width: 24rem;
    text-align: center;
  }

  .empty-symbol {
    align-items: center;
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-control);
    color: var(--brand-action-text);
    display: flex;
    height: 2.75rem;
    justify-content: center;
    margin-bottom: var(--space-1);
    width: 2.75rem;
  }

  strong {
    color: var(--text-primary);
    font-size: var(--font-size-body);
    line-height: 1.25;
  }

  .table-empty-state > span:not(.empty-symbol) {
    font-size: var(--font-size-meta);
    line-height: 1.4;
  }

  button {
    margin-top: var(--space-1);
  }
</style>

<script lang="ts">
  import type { Snippet } from 'svelte';

  const {
    id,
    title,
    description,
    actions,
  }: {
    id: string;
    title: string;
    description: string;
    actions?: Snippet;
  } = $props();
</script>

<header class="panel-header">
  <div class="panel-heading">
    <h2 {id}>{title}</h2>
    <p>{description}</p>
  </div>
  {#if actions !== undefined}
    <div class="panel-header-actions">
      {@render actions()}
    </div>
  {/if}
</header>

<style>
  .panel-header {
    align-items: center;
    display: flex;
    gap: var(--space-6);
    justify-content: space-between;
    min-height: 5.25rem;
    padding: var(--space-2) 0 var(--space-6);
  }

  .panel-heading {
    min-width: 0;
  }

  h2 {
    font-size: clamp(1.5rem, 2vw, 1.75rem);
    font-weight: 700;
    letter-spacing: -0.035em;
    line-height: 1.2;
    margin: 0;
  }

  p {
    color: var(--text-secondary);
    font-size: var(--font-size-body);
    line-height: 1.5;
    margin: var(--space-2) 0 0;
    max-width: 52rem;
    text-wrap: balance;
  }

  .panel-header-actions {
    align-items: center;
    display: flex;
    flex: none;
    gap: var(--space-2);
    margin-left: auto;
  }

  @media (max-width: 36rem) {
    .panel-header {
      align-items: stretch;
      flex-direction: column;
      gap: var(--space-3);
      padding-bottom: var(--space-4);
    }

    .panel-header-actions {
      margin-left: 0;
      width: 100%;
    }
  }
</style>

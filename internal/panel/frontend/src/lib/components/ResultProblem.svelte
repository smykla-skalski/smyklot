<script lang="ts">
  import Icon from './Icon.svelte';

  const {
    title,
    problem,
    onRetry,
    busy = false,
    overContent = false,
  }: {
    /** What could not be read, as a sentence: "Repositories could not be loaded". */
    title: string;
    problem: string;
    onRetry: () => void;
    busy?: boolean;
    /**
     * Whether readable content sits behind this. A refresh that fails over a
     * loaded table has not made the table wrong, so the failure becomes a line
     * above it rather than a screen in place of it.
     */
    overContent?: boolean;
  } = $props();
</script>

{#snippet retry()}
  <button class="btn" type="button" onclick={onRetry} disabled={busy}>
    {busy ? 'Trying again…' : 'Try again'}
  </button>
{/snippet}

{#if overContent}
  <div class="result-notice" role="alert">
    <span class="notice-copy">
      <strong>{title}</strong>
      <span>{problem}</span>
    </span>
    {@render retry()}
  </div>
{:else}
  <!-- The same shape the empty state has, because it stands in the same place and
       answers the same question. Only the mark's tone and glyph differ: this one
       is a thing that went wrong rather than a thing that is not there. -->
  <div class="table-notice" role="alert">
    <span class="table-notice-mark alarmed" aria-hidden="true"
      ><Icon name="warning" size={22} /></span
    >
    <strong>{title}</strong>
    <span>{problem}</span>
    {@render retry()}
  </div>
{/if}

<style>
  /* Over content: one line, sized to itself, so what is already on screen keeps
     its place. */
  .result-notice {
    align-items: center;
    background: var(--stop-tint);
    border-radius: var(--r-ctl);
    color: var(--stop);
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2) var(--space-3);
    justify-content: space-between;
    margin: var(--space-3) var(--space-3) 0;
    padding: var(--space-2) var(--space-3);
  }

  .notice-copy {
    display: flex;
    flex-wrap: wrap;
    gap: 0 var(--space-2);
    min-width: 0;
  }

  .result-notice strong,
  .notice-copy span {
    font-size: var(--font-size-meta);
  }

  .result-notice .btn {
    flex: none;
  }
</style>

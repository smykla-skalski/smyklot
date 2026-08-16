<script lang="ts">
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
  <div class="result-state" role="alert">
    <strong>{title}</strong>
    <span>{problem}</span>
    {@render retry()}
  </div>
{/if}

<style>
  /* Nothing loaded: the failure is the view, so it fills the space the content
     would have taken and does not resize when it arrives. */
  .result-state {
    align-items: center;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    justify-content: center;
    min-height: 10rem;
    padding: var(--space-6);
    text-align: center;
  }

  .result-state span {
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
  }

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

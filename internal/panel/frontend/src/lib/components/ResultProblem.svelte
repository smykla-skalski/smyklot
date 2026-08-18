<script lang="ts">
  import Button from './Button.svelte';
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
  <Button onclick={onRetry} disabled={busy}>
    {busy ? 'Trying again…' : 'Try again'}
  </Button>
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

  /* `:global` because `Button` renders the control, so it carries that component's
     scope class rather than this one's and a plain `.btn` would stop matching. The
     left-hand side still carries this component's scope, so the rule reaches no
     button but the one in this notice. */
  .result-notice :global(.btn) {
    flex: none;
  }
</style>

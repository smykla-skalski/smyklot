<script lang="ts">
  import Button from './Button.svelte';

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

<!--
@component
A request that failed, and the retry. Distinct from `EmptyState`, which is a
collection that succeeded and holds nothing: an empty list is not an error and drawing
it as one teaches a reader to distrust the page.

`overContent` is the whole design. A refresh that fails over a table that is already
loaded has not made that table wrong, so the failure becomes a line ABOVE the rows
rather than a screen in place of them - the reader keeps what they were reading, and
the rows already fetched stay exactly where they are.

`busy` is the retry reporting on itself, so a second press cannot be sent while the
first is still in flight.
-->

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
       answers the same question. `is-error` is the whole difference: a thing that
       went wrong rather than a thing that is not there. -->
  <div class="state-panel is-error" role="alert">
    <span><strong>{title}.</strong> {problem}</span>
    {@render retry()}
  </div>
{/if}

<style>
  /* Over content: one line, sized to itself, so what is already on screen keeps
     its place. */
  .result-notice {
    align-items: center;
    background: var(--danger-tint);
    border-radius: var(--r-ctl);
    color: var(--danger);
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

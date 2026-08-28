<script lang="ts">
  import Button from './Button.svelte';

  const {
    count,
    saving = false,
    resolving = false,
    problem = null,
    invalidProblem = null,
    problemHref,
    problemLabel,
    notice = null,
    conflict = false,
    readOnly = false,
    onSave,
    onDiscard,
    onResolveConflict,
    onDismiss,
    onOpenProblem,
  }: {
    count: number;
    saving?: boolean;
    resolving?: boolean;
    problem?: string | null;
    invalidProblem?: string | null;
    problemHref?: string;
    problemLabel?: string;
    notice?: string | null;
    conflict?: boolean;
    readOnly?: boolean;
    onSave: () => void;
    onDiscard: () => void;
    onResolveConflict: () => void;
    onDismiss: () => void;
    onOpenProblem?: () => void;
  } = $props();

  const noun = $derived(count === 1 ? 'setting' : 'settings');

  function openProblem(event: MouseEvent): void {
    if (onOpenProblem === undefined) return;
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    onOpenProblem();
  }
</script>

{#if count > 0 || saving || resolving || problem !== null || notice !== null}
  <aside class="settings-composer" aria-label="Settings draft">
    <div class="composer-copy" aria-live="polite">
      {#if resolving}
        <strong>Updating your draft…</strong>
        <span>Your unsaved decisions will stay in place</span>
      {:else if saving}
        <strong>Saving settings…</strong>
        <span>Every changed setting in this workspace will land together</span>
      {:else if invalidProblem !== null}
        <strong>Fix the invalid setting before saving</strong>
        <span>{invalidProblem}</span>
      {:else if problem !== null || conflict}
        <strong>{conflict ? 'Your draft is still safe' : 'Settings were not saved'}</strong>
        <span>{problem ?? 'Settings also changed in another open tab'}</span>
        {#if problemHref !== undefined && problemLabel !== undefined}
          <a href={problemHref} onclick={openProblem}>Open {problemLabel}</a>
        {/if}
      {:else if notice !== null}
        <strong>Settings saved</strong>
        <span>{notice}</span>
      {:else}
        <strong>{count} changed {noun}</strong>
        <span>Review anywhere in this workspace, then save everything together</span>
      {/if}
    </div>

    <div class="composer-actions">
      {#if count > 0}
        <Button disabled={saving || resolving} onclick={onDiscard}>Discard</Button>
        {#if conflict}
          <Button tone="signal" disabled={resolving} onclick={onResolveConflict}>
            {resolving ? 'Updating…' : 'Update draft'}
          </Button>
        {:else}
          <Button
            tone="signal"
            disabled={saving || resolving || readOnly || invalidProblem !== null}
            onclick={onSave}
          >
            {saving ? 'Saving…' : 'Save'}
          </Button>
        {/if}
      {:else}
        <Button onclick={onDismiss}>Dismiss</Button>
      {/if}
    </div>
  </aside>
{/if}

<style>
  .settings-composer {
    align-items: center;
    animation: composer-arrive 160ms ease-out both;
    backdrop-filter: blur(14px);
    background: color-mix(in srgb, var(--surface-base) 92%, transparent);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    bottom: max(var(--space-4), env(safe-area-inset-bottom));
    box-shadow: var(--shadow-plate);
    display: flex;
    gap: var(--space-5);
    justify-content: space-between;
    left: 50%;
    max-width: 50rem;
    padding: var(--space-3) var(--space-4);
    position: fixed;
    transform: translateX(-50%);
    width: calc(100vw - 2 * var(--space-4));
    z-index: 30;
  }

  .composer-copy {
    display: grid;
    gap: var(--space-1);
    min-width: 0;
  }

  .composer-copy strong,
  .composer-copy span {
    overflow-wrap: anywhere;
  }

  .composer-copy span {
    color: var(--dim);
    font-size: var(--font-size-compact);
  }

  .composer-copy a {
    color: var(--accent);
    font-size: var(--font-size-compact);
    justify-self: start;
  }

  .composer-actions {
    display: flex;
    flex: 0 0 auto;
    gap: var(--space-2);
  }

  @keyframes composer-arrive {
    from {
      opacity: 0;
      transform: translate(-50%, 0.5rem);
    }
  }

  @media (max-width: 42rem) {
    .settings-composer {
      align-items: stretch;
      flex-direction: column;
      gap: var(--space-3);
    }

    .composer-actions {
      justify-content: flex-end;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .settings-composer {
      animation: none;
    }
  }
</style>

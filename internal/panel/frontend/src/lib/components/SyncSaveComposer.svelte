<script lang="ts">
  import type { SyncDraftSet } from '#lib/sync-drafts.svelte.js';
  import type { SyncKind } from '#lib/types.js';

  import Button from './Button.svelte';

  const {
    drafts,
    readOnly,
    onSave,
    onReload,
    sectionHref,
    onOpenSection,
  }: {
    drafts: SyncDraftSet;
    readOnly: boolean;
    onSave: () => void;
    onReload: () => void;
    sectionHref: (kind: SyncKind) => string;
    onOpenSection: (kind: SyncKind) => void;
  } = $props();

  const count = $derived(drafts.dirtyCount);
  const noun = $derived(count === 1 ? 'section' : 'sections');

  function openKind(event: MouseEvent, kind: SyncKind): void {
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    onOpenSection(kind);
  }
</script>

{#if count > 0 || drafts.saving || drafts.problem !== null || drafts.notice !== null}
  <aside class="sync-composer" aria-label="Sync configuration draft">
    <div class="composer-copy" aria-live="polite">
      {#if drafts.refreshing}
        <strong>Loading latest Sync configuration…</strong>
        <span>Your draft will stay unchanged.</span>
      {:else if drafts.saving}
        <strong>Saving Sync configuration…</strong>
        <span>All changed sections will land together.</span>
      {:else if drafts.problem !== null}
        <strong
          >{drafts.conflict
            ? 'Your draft is still safe'
            : 'Sync configuration was not saved'}</strong
        >
        <span>{drafts.problem}</span>
        {#if drafts.invalidKind !== null}
          <a
            href={sectionHref(drafts.invalidKind)}
            onclick={(event) => openKind(event, drafts.invalidKind!)}
          >
            Open {drafts.invalidKind}
          </a>
        {/if}
      {:else if drafts.notice !== null}
        <strong>Sync configuration saved</strong>
        <span>{drafts.notice}</span>
      {:else}
        <strong>{count} changed Sync {noun}</strong>
        <span>Review anywhere in this installation, then save everything together.</span>
      {/if}
    </div>

    <div class="composer-actions">
      {#if count > 0}
        <Button disabled={drafts.saving || drafts.refreshing} onclick={() => drafts.discard()}
          >Discard</Button
        >
        {#if drafts.conflict}
          <Button tone="signal" disabled={drafts.refreshing} onclick={onReload}>
            {drafts.refreshing ? 'Loading…' : 'Load latest'}
          </Button>
        {:else}
          <Button
            tone="signal"
            disabled={drafts.saving || drafts.refreshing || readOnly}
            onclick={onSave}
          >
            {drafts.saving ? 'Saving…' : 'Save'}
          </Button>
        {/if}
      {:else}
        <Button onclick={() => drafts.dismissNotice()}>Dismiss</Button>
      {/if}
    </div>
  </aside>
{/if}

<style>
  .sync-composer {
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
    .sync-composer {
      align-items: stretch;
      flex-direction: column;
      gap: var(--space-3);
    }

    .composer-actions {
      justify-content: flex-end;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .sync-composer {
      animation: none;
    }
  }
</style>

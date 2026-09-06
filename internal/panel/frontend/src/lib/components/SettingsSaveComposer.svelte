<script lang="ts">
  import { MediaQuery } from 'svelte/reactivity';
  import { fly } from 'svelte/transition';

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
  const needsAction = $derived(
    count > 0 || problem !== null || invalidProblem !== null || conflict,
  );
  const isReceipt = $derived(!needsAction && !saving && !resolving && notice !== null);
  const visible = $derived(needsAction || saving || resolving || isReceipt);
  const canDismiss = $derived(
    !saving && !resolving && invalidProblem === null && (isReceipt || problem !== null),
  );
  const reducedMotion =
    typeof window !== 'undefined' && typeof window.matchMedia === 'function'
      ? new MediaQuery('prefers-reduced-motion: reduce')
      : null;
  let hovered = $state(false);
  let focused = $state(false);

  // Only a settled receipt expires. Any new draft, problem, save or notice cancels
  // its timer, so an old receipt can never dismiss work that arrived after it.
  $effect(() => {
    const receipt = notice;
    if (!isReceipt || hovered || focused) return;
    const timer = setTimeout(() => {
      if (isReceipt && notice === receipt && !hovered && !focused) onDismiss();
    }, 5_000);
    return () => clearTimeout(timer);
  });

  function leaveFocus(event: FocusEvent): void {
    focused =
      event.relatedTarget instanceof Node &&
      (event.currentTarget as HTMLElement).contains(event.relatedTarget);
  }

  function openProblem(event: MouseEvent): void {
    if (onOpenProblem === undefined) return;
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    onOpenProblem();
  }
</script>

<!--
@component
The bar that appears once a settings page has unsaved changes, and the only thing on
these pages that writes. Every control on a settings page stages a draft; this is what
sends it, which is why a switch can be flipped without anything happening yet.

It says how many changes are waiting, because "unsaved changes" without a count leaves
a reader to go looking for what they touched.

`conflict` is the case worth knowing: somebody else saved while this draft was open, so
the choice is no longer save-or-discard but reconcile. `readOnly` keeps the bar visible
and its actions closed - a reader who cannot save still needs to see that they have
changed something.
-->

{#if visible}
  <aside
    class="settings-composer"
    class:action-required={needsAction || saving || resolving}
    aria-label={isReceipt ? 'Settings receipt' : 'Settings draft'}
    onpointerenter={() => (hovered = true)}
    onpointerleave={() => (hovered = false)}
    onfocusin={() => (focused = true)}
    onfocusout={leaveFocus}
    in:fly={{ y: 8, duration: reducedMotion?.current !== false ? 0 : 180 }}
    out:fly={{ y: 8, duration: reducedMotion?.current !== false ? 0 : 140 }}
  >
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
      {:else if isReceipt}
        <strong>Settings saved</strong>
        <span>{notice}</span>
      {:else}
        <strong>{count} changed {noun}</strong>
        <span>Review anywhere in this workspace, then save everything together</span>
      {/if}
    </div>

    {#if count > 0 || canDismiss}
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
    {/if}
  </aside>
{/if}

<style>
  .settings-composer {
    align-items: center;
    background: var(--popover-bg);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    bottom: max(var(--space-4), env(safe-area-inset-bottom));
    box-shadow: var(--shadow-plate);
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3) var(--space-5);
    justify-content: space-between;
    /* The navigation keeps its own space. Both insets and auto margins center
       the composer in the remaining pane, including the collapsed sidebar. */
    inset-inline: calc(var(--pane-chrome-width, 0px) + var(--space-4)) var(--space-4);
    margin-inline: auto;
    max-width: 50rem;
    padding: var(--space-3) var(--space-4);
    position: fixed;
    z-index: var(--layer-sticky);
  }

  .action-required {
    --brand-action: var(--decision-accent);
    --brand-action-hover: var(--decision-accent-hover);
    --brand-action-pressed: var(--decision-accent-pressed);
    --on-brand-action: var(--on-decision-accent);
    border-color: var(--decision-accent);
    border-width: var(--decision-border-width);
  }

  .composer-copy {
    display: grid;
    flex: 1 1 18rem;
    gap: var(--row-copy-gap);
    min-width: 0;
  }

  .composer-copy strong,
  .composer-copy span {
    line-height: var(--row-copy-leading);
    overflow-wrap: anywhere;
    text-box: trim-both cap alphabetic;
  }

  .composer-copy span {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
  }

  .composer-copy a {
    color: var(--brand-action);
    font-size: var(--font-size-compact);
    justify-self: start;
  }

  .composer-actions {
    display: flex;
    flex: 0 0 auto;
    gap: var(--space-2);
    margin-inline-start: auto;
  }

  @media (max-width: 42rem) {
    .settings-composer {
      align-items: stretch;
    }

    .composer-actions {
      justify-content: flex-end;
    }
  }
</style>

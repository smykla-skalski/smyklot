<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { formatDate, formatTimestamp } from '../format';
  import type { AccessDecision } from '../types';
  import Avatar from './Avatar.svelte';
  import Chip, { type ChipTone } from './Chip.svelte';
  import Modal from './Modal.svelte';

  const {
    open,
    label,
    scopeLabel,
    status,
    reason,
    decidedAt,
    returnFocus = null,
    queryKey,
    fetchDecisions,
    onClose,
  }: {
    open: boolean;
    label: string;
    scopeLabel: string;
    status: string;
    reason?: string;
    decidedAt?: string;
    returnFocus?: HTMLElement | null;
    queryKey: readonly unknown[];
    fetchDecisions: () => Promise<AccessDecision[]>;
    onClose: () => void;
  } = $props();

  const decisionsQuery = createQuery(() => ({
    queryKey: [...queryKey],
    queryFn: fetchDecisions,
    enabled: open,
  }));
  const decisions = $derived<AccessDecision[] | null>(decisionsQuery.data ?? null);
  const loading = $derived(decisionsQuery.isFetching);
  const failure = $derived(
    decisionsQuery.error === null
      ? null
      : decisionsQuery.error instanceof Error
        ? decisionsQuery.error.message
        : String(decisionsQuery.error),
  );

  function statusTone(value: string): ChipTone {
    const normalized = value.toLowerCase();
    if (normalized === 'banned') return 'stop';
    if (normalized === 'suspended') return 'warning';
    if (normalized === 'active' || normalized === 'allowed') return 'clear';
    return 'neutral';
  }

  // Audit actions are dotted paths ("target.access.suspended"); the final
  // segment names the resulting state the chip reports.
  function decisionState(action: string): string {
    return action.split('.').at(-1) ?? action;
  }

  function decisionLabel(action: string): string {
    const state = decisionState(action);
    return state.charAt(0).toUpperCase() + state.slice(1);
  }

  function decisionTone(action: string): ChipTone {
    const state = decisionState(action);
    if (state === 'banned' || state === 'removed' || state === 'revoked') return 'stop';
    if (state === 'suspended' || state === 'expired') return 'warning';
    if (['accepted', 'created', 'readded', 'restored', 'unbanned'].includes(state)) return 'clear';
    return 'neutral';
  }

  function close(): void {
    onClose();
  }
</script>

<Modal
  id="decision-history"
  {open}
  title={label}
  description="Current state and earlier administrator decisions"
  {returnFocus}
  onClose={close}
>
  <dl class="current-decision">
    <div>
      <dt>Status</dt>
      <dd><Chip tone={statusTone(status)} dot>{status}</Chip></dd>
    </div>
    <div>
      <dt>Scope</dt>
      <dd title={scopeLabel}>Workspace</dd>
    </div>
    <div>
      <dt>Decided</dt>
      <dd>
        {#if decidedAt !== undefined}
          <time datetime={decidedAt} title={formatTimestamp(decidedAt)}>
            {formatDate(decidedAt)}
          </time>
        {:else}
          <span class="dim">Unknown</span>
        {/if}
      </dd>
    </div>
  </dl>

  {#if reason !== undefined && reason.trim() !== ''}
    <section class="current-reason" aria-labelledby="decision-reason-heading">
      <h3 id="decision-reason-heading">Reason</h3>
      <p>{reason}</p>
    </section>
  {/if}

  <section class="history-section" aria-labelledby="decision-history-heading">
    <h3 id="decision-history-heading">Decision history</h3>
    <div class="decision-list" aria-live="polite">
      {#if loading && decisions === null}
        <p class="state dim">Loading decisions…</p>
      {:else if failure !== null}
        <p class="state form-error" role="alert">{failure}</p>
      {:else}
        {#each decisions ?? [] as decision (decision.id)}
          <article>
            <Avatar account={decision.actor} size={28} />
            <span class="decision-copy">
              <strong class="cap-trim" title={decision.summary}>
                {decisionLabel(decision.action)} by @{decision.actor.login}
              </strong>
              <span class="decision-meta cap-trim">
                {#each decision.summary.split(/(Audit #\d+)/) as part, partIndex (partIndex)}
                  {#if /^Audit #\d+$/.test(part)}<span class="audit-ref mono">{part}</span
                    >{:else}{part}{/if}
                {/each}
              </span>
            </span>
            <Chip tone={decisionTone(decision.action)}>{decisionLabel(decision.action)}</Chip>
            <time
              class="decision-date mono"
              datetime={decision.created_at}
              title={formatTimestamp(decision.created_at)}
            >
              {formatDate(decision.created_at)}
            </time>
          </article>
        {:else}
          <p class="state dim">No earlier decisions in this scope</p>
        {/each}
      {/if}
    </div>
  </section>

  {#snippet footer()}
    <button class="btn btn-ghost" type="button" data-modal-focus onclick={close}>Close</button>
  {/snippet}
</Modal>

<style>
  .current-decision {
    background: var(--well);
    border: 1px solid var(--rule);
    border-radius: var(--r-well);
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    margin: 0;
  }

  .current-decision > div {
    min-width: 0;
    padding: var(--space-3);
  }

  .current-decision > div + div {
    border-inline-start: 1px solid var(--rule);
  }

  /* 1.3, not the inherited 1.5: an uppercase micro key has no descenders to
     clear, and the extra leading is what made these boxes stand a couple of
     pixels taller than the cards they sit beside. */
  dt,
  h3 {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
    font-weight: 700;
    letter-spacing: 0.05em;
    line-height: 1.3;
    text-transform: uppercase;
  }

  dd {
    align-items: center;
    display: flex;
    font-size: var(--font-size-meta);
    font-weight: 600;
    gap: var(--space-2);
    margin: 0.4rem 0 0;
    min-width: 0;
  }

  dd time {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .current-reason {
    background: var(--warning-tint);
    border: 1px solid color-mix(in srgb, var(--warning) 35%, transparent);
    border-radius: var(--r-well);
    padding: var(--space-3);
  }

  h3 {
    margin: 0;
  }

  /* Only the boxed "Reason" key is an uppercase micro header. "Decision history"
     labels a list rather than a value, so the mock sets it in sentence case at
     compact size. */
  .history-section h3 {
    color: var(--text-secondary);
    font: 650 var(--font-size-compact) / 1 var(--sans);
    letter-spacing: normal;
    margin-bottom: 0.5rem;
    text-transform: none;
  }

  .current-reason h3 {
    color: var(--warning);
  }

  .current-reason p {
    font-size: var(--font-size-meta);
    line-height: 1.5;
    margin: 0.3rem 0 0;
  }

  .decision-list {
    border: 1px solid var(--rule);
    border-radius: var(--r-well);
    margin-top: var(--space-2);
    max-height: min(21rem, 42vh);
    overflow: auto;
  }

  article {
    align-items: center;
    display: grid;
    gap: var(--space-3);
    grid-template-columns: auto minmax(0, 1fr) auto auto;
    padding: var(--space-3);
  }

  article + article {
    border-top: 1px solid var(--rule);
  }

  /* Both cap-trimmed lines drop their leading, so the block's box equals its
     ink and the grid's vertical centering lands optically against the avatar. */
  .decision-copy {
    min-width: 0;
  }

  /* No overflow clip here: cap-trim shortens the line box to the cap height, so
     `overflow: hidden` sliced the descenders off ("Banned by" read "Banned bv").
     The mock lets the line wrap instead. */
  article strong {
    display: block;
    font-size: var(--font-size-meta);
    font-weight: 700;
    line-height: 1;
    min-width: 0;
  }

  .decision-meta {
    color: var(--text-muted);
    display: block;
    font: 400 var(--font-size-compact) / 1 var(--sans);
    margin-top: 0.4rem;
  }

  .decision-date {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    font-weight: 500;
    line-height: 1;
    min-width: 4.5rem;
    text-box: trim-both cap alphabetic;
    white-space: nowrap;
  }

  .audit-ref {
    color: var(--brand-action-text);
  }

  .state {
    font-size: var(--font-size-compact);
    margin: 0;
    padding: var(--space-4);
    text-align: center;
  }

  @media (max-width: 32rem) {
    .current-decision {
      grid-template-columns: minmax(0, 1fr);
    }

    .current-decision > div + div {
      border-inline-start: 0;
      border-top: 1px solid var(--rule);
    }

    article {
      align-items: start;
      grid-template-columns: auto minmax(0, 1fr);
    }

    article :global(.chip),
    .decision-date {
      grid-column: 2;
    }
  }
</style>

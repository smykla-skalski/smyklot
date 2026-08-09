<script lang="ts">
  import { onMount } from 'svelte';
  import { formatDateTime, formatRelative, formatTimestamp } from '../lib/format';
  import type { AccessDecision } from '../lib/types';
  import Avatar from './Avatar.svelte';
  import Modal from './Modal.svelte';

  const {
    open,
    label,
    scopeLabel,
    status,
    reason,
    decidedAt,
    returnFocus = null,
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
    fetchDecisions: () => Promise<AccessDecision[]>;
    onClose: () => void;
  } = $props();

  let decisions = $state.raw<AccessDecision[] | null>(null);
  let loading = $state(false);
  let failure = $state<string | null>(null);
  const now = Date.now();

  onMount(() => {
    let active = true;
    loading = true;
    void fetchDecisions()
      .then((listed) => {
        if (active) decisions = listed;
      })
      .catch((error: unknown) => {
        if (active) failure = error instanceof Error ? error.message : String(error);
      })
      .finally(() => {
        if (active) loading = false;
      });
    return () => {
      active = false;
    };
  });

  function close(): void {
    onClose();
  }
</script>

<Modal
  id="decision-history"
  {open}
  title={label}
  description="Review the current access state and earlier administrator decisions"
  variant="wide"
  {returnFocus}
  onClose={close}
>
  <dl class="current-decision">
    <div>
      <dt>Status</dt>
      <dd>{status}</dd>
    </div>
    <div>
      <dt>Scope</dt>
      <dd>{scopeLabel}</dd>
    </div>
    <div>
      <dt>Decided</dt>
      <dd>
        {#if decidedAt !== undefined}
          <time datetime={decidedAt} title={formatTimestamp(decidedAt)}>
            {formatDateTime(decidedAt)}
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
      {#if loading}
        <p class="state dim">Loading decisions…</p>
      {:else if failure !== null}
        <p class="state form-error" role="alert">{failure}</p>
      {:else}
        {#each decisions ?? [] as decision (decision.id)}
          <article>
            <Avatar account={decision.actor} size={28} />
            <strong title={decision.summary}>{decision.summary}</strong>
            <span class="decision-meta">
              {decision.actor.display_name} ·
              <time datetime={decision.created_at} title={formatTimestamp(decision.created_at)}>
                {formatRelative(decision.created_at, now)}
              </time>
            </span>
            <code>{decision.action}</code>
          </article>
        {:else}
          <p class="state dim">No earlier decisions in this scope</p>
        {/each}
      {/if}
    </div>
  </section>

  {#snippet footer()}
    <button class="btn" type="button" data-modal-focus onclick={close}>Close</button>
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
    padding: 0.75rem;
  }

  .current-decision > div + div {
    border-inline-start: 1px solid var(--rule);
  }

  dt,
  h3 {
    color: var(--text-secondary);
    font: 650 var(--font-size-compact) / 1.2 var(--sans);
    letter-spacing: 0.02em;
  }

  dd {
    align-items: center;
    display: flex;
    font-size: 0.75rem;
    font-weight: 600;
    gap: 0.4rem;
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
    margin-top: 0.75rem;
    padding: 0.75rem;
  }

  h3 {
    margin: 0;
  }

  .current-reason h3 {
    color: var(--warning);
  }

  .current-reason p {
    font-size: 0.75rem;
    line-height: 1.45;
    margin: 0.3rem 0 0;
  }

  .history-section {
    margin-top: 1rem;
  }

  .decision-list {
    border: 1px solid var(--rule);
    border-radius: var(--r-well);
    margin-top: 0.5rem;
    max-height: min(21rem, 42vh);
    overflow: auto;
  }

  article {
    align-items: center;
    display: grid;
    gap: 0.625rem;
    grid-template-columns: auto minmax(10rem, 1fr) max-content max-content;
    padding: 0.7rem;
  }

  article + article {
    border-top: 1px solid var(--rule);
  }

  article strong {
    font-size: 0.75rem;
    line-height: 1.3;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .decision-meta {
    color: var(--dim);
    font-size: 0.6875rem;
    line-height: 1.3;
    white-space: nowrap;
  }

  article code {
    font-size: 0.5625rem;
    white-space: nowrap;
  }

  .state {
    font-size: 0.75rem;
    margin: 0;
    padding: 1rem;
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

    article strong,
    .decision-meta,
    article code {
      grid-column: 2;
    }

    .decision-meta {
      white-space: normal;
    }
  }
</style>

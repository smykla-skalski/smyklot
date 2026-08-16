<script lang="ts">
  import { formatTimestamp } from '../format';
  import type { PendingCIDetail, PendingCIEvent, PendingCITrigger } from '../types';
  import Icon from './Icon.svelte';

  const {
    detail,
    loading,
    error,
  }: {
    detail: PendingCIDetail | null;
    loading: boolean;
    error: string | null;
  } = $props();

  function words(value: string): string {
    return value.replaceAll('_', ' ');
  }

  function triggerLabel(trigger: PendingCITrigger): string {
    switch (trigger) {
      case 'webhook':
        return 'Webhook';
      case 'fallback':
        return 'Fallback poll';
      case 'quiet_period':
        return 'Quiet-period timer';
      case 'manual':
        return 'Panel';
      case 'command':
        return 'Command';
      case 'cleanup':
        return 'Cleanup';
    }
  }

  function eventTitle(event: PendingCIEvent): string {
    switch (event.kind) {
      case 'wake_received':
        return event.event_name ? `${event.event_name} received` : 'Wake received';
      case 'reconciliation_started':
        return 'Reconciliation started';
      case 'checks_observed':
        return event.state ? `CI observed: ${words(event.state)}` : 'CI observed';
      case 'merge_started':
        return 'Merge started';
      case 'cleanup_retry':
        return 'Cleanup will retry';
      case 'cleanup_completed':
        return 'Cleanup completed';
      default:
        return words(event.kind);
    }
  }
</script>

<div class="audit" aria-busy={loading} aria-live="polite">
  {#if loading && detail === null && error === null}
    <p class="audit-state"><Icon name="pending" size={14} /> Loading operational timeline…</p>
  {:else if error !== null}
    <p class="audit-state error" role="alert"><Icon name="failure" size={14} /> {error}</p>
  {:else if detail !== null}
    {#if detail.events.length === 0}
      <p class="audit-state">No durable audit events were recorded for this request.</p>
    {:else}
      <ol>
        {#each detail.events as event (event.id)}
          <li>
            <span class:terminal={event.kind === 'finished'} class="event-dot"></span>
            <div class="event-body">
              <div class="event-heading">
                <strong>{eventTitle(event)}</strong>
                <time datetime={event.created_at}>{formatTimestamp(event.created_at)}</time>
              </div>
              <p>{event.summary}</p>
              <div class="event-facts">
                <span>{triggerLabel(event.trigger)}</span>
                {#if event.delivery_id}
                  <code title="GitHub delivery ID">delivery {event.delivery_id}</code>
                {/if}
                {#if event.event_key}
                  <code title={event.event_key}>{event.event_key}</code>
                {/if}
              </div>
            </div>
          </li>
        {/each}
      </ol>
    {/if}
  {/if}
</div>

<style>
  .audit {
    background: color-mix(in srgb, var(--surface-raised) 62%, transparent);
    border-top: 1px solid var(--border-subtle);
    grid-column: 1 / -1;
    margin-top: var(--space-2);
    padding: var(--space-4);
  }

  .audit-state {
    align-items: center;
    color: var(--text-secondary);
    display: flex;
    font-size: var(--font-size-compact);
    gap: var(--space-2);
    margin: 0;
  }

  .audit-state.error {
    color: var(--stop);
  }

  ol {
    display: grid;
    gap: var(--space-3);
    list-style: none;
    margin: 0;
    padding: 0;
  }

  li {
    display: grid;
    gap: var(--space-3);
    grid-template-columns: 0.65rem minmax(0, 1fr);
  }

  .event-dot {
    background: var(--accent);
    border-radius: 50%;
    height: 0.5rem;
    margin-top: 0.35rem;
    width: 0.5rem;
  }

  .event-dot.terminal {
    background: var(--text-primary);
  }

  .event-heading,
  .event-facts {
    align-items: baseline;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .event-heading {
    justify-content: space-between;
  }

  .event-heading strong {
    font-size: var(--font-size-meta);
    text-transform: capitalize;
  }

  .event-heading time,
  .event-facts,
  .event-body p {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
  }

  .event-body p {
    margin: 0.2rem 0;
  }

  .event-facts code {
    max-width: 24rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>

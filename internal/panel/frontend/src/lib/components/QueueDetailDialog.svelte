<script lang="ts">
  import { formatDateTime } from '#lib/format.js';
  import type { QueueDetail, QueueItem } from '#lib/types.js';
  import { workloadTitle } from '#lib/workloads.js';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';

  const {
    open,
    detail,
    loading,
    error,
    onClose,
  }: {
    open: boolean;
    detail: QueueDetail | null;
    loading: boolean;
    error: string;
    onClose: () => void;
  } = $props();

  function words(value: string): string {
    return value.replaceAll('_', ' ').replace(/^./, (letter) => letter.toUpperCase());
  }

  /** Seconds, because this dialog is where two events are put in order. */
  const absolute = (value: string, timeZone?: string): string =>
    formatDateTime(value, { timeZone, named: true, seconds: true });

  function scope(item: QueueItem): string {
    if (item.repository_id) return `${item.target_id ?? 'Global'} / ${item.repository_id}`;
    return item.target_id ?? 'Global';
  }
</script>

<!--
@component
One piece of queued work, read alongside the queue rather than instead of it. An
inspector, which is the widest of the three dialog shapes and the one that does not
cover the list it came from: the reader is comparing this record against its
neighbours, so taking the neighbours away would be the wrong help.

`loading` and `error` are the dialog's own, not the queue's. It opens on a row that is
already on screen and fetches the rest, so it has a state the list behind it does
not.
-->

<Modal
  id="queue-detail"
  {open}
  variant="inspector"
  title={detail?.item.title ?? 'Queue item'}
  description="When it runs, what it has done, and every change it has been through"
  {onClose}
>
  {#if loading && detail === null}
    <p class="detail-message" aria-live="polite">Loading queue item…</p>
  {:else if error !== ''}
    <p class="detail-message detail-error" role="alert">{error}</p>
  {:else if detail !== null}
    <dl class="facts">
      <div>
        <dt>Job</dt>
        <dd>{workloadTitle(detail.item.kind)}</dd>
      </div>
      <div>
        <dt>State</dt>
        <dd>{words(detail.item.state)}</dd>
      </div>
      <div>
        <dt>Scope</dt>
        <dd>{scope(detail.item)}</dd>
      </div>
      <div>
        <dt>Priority</dt>
        <dd>{words(detail.item.priority)}</dd>
      </div>
      <div>
        <dt>Hours</dt>
        <dd>
          {detail.item.profile_name ?? 'Immediate'}{detail.item.profile_timezone
            ? ` · ${detail.item.profile_timezone}`
            : ''}
        </dd>
      </div>
      <div>
        <dt>Ready, in your timezone</dt>
        <dd>{absolute(detail.item.eligible_at)}</dd>
      </div>
      {#if detail.item.profile_timezone}
        <div>
          <dt>Ready, in the job's timezone</dt>
          <dd>{absolute(detail.item.eligible_at, detail.item.profile_timezone)}</dd>
        </div>
      {/if}
      <div>
        <dt>Estimated start</dt>
        <dd>
          {detail.item.estimated_start_at
            ? `${absolute(detail.item.estimated_start_at)} · estimate`
            : 'Not estimated'}
        </dd>
      </div>
      <div>
        <dt>Work ahead</dt>
        <dd>{detail.item.work_ahead}</dd>
      </div>
      <div>
        <dt>Attempts</dt>
        <dd>{detail.item.attempt}</dd>
      </div>
      <div>
        <dt>Revision</dt>
        <dd>{detail.item.revision}</dd>
      </div>
    </dl>

    <section class="workload-detail" aria-labelledby="queue-workload-detail">
      <h3 id="queue-workload-detail">What this job is doing</h3>
      {#if detail.item.kind === 'webhook_delivery'}
        {#if detail.item.details}
          <p>
            Event {detail.item.details.event ?? 'unknown'} · delivery {detail.item.details
              .delivery_id ?? 'unknown'}
          </p>
        {:else}
          <p>Webhook payload and delivery failure detail are protected.</p>
        {/if}
      {:else if detail.item.kind === 'pending_ci'}
        <p>
          Pull request {detail.item.details?.pull_request ?? 'unknown'} · head
          {detail.item.details?.head_sha?.slice(0, 12) ?? 'unknown'}
        </p>
      {:else if detail.item.kind === 'sync_apply'}
        <p>
          {detail.item.details?.create ?? 0} create · {detail.item.details?.update ?? 0} update ·
          {detail.item.details?.delete ?? 0} delete
        </p>
      {:else if detail.item.kind === 'schedule_change'}
        <p>
          The job asked about: {detail.item.details?.policy_kind === undefined
            ? 'unknown'
            : workloadTitle(detail.item.details.policy_kind)}
        </p>
      {:else}
        <p>{detail.item.summary ?? 'Nothing further was recorded about this job.'}</p>
      {/if}
      {#if detail.item.blocked_reason}
        <p class="blocking"><strong>Blocked:</strong> {detail.item.blocked_reason}</p>
      {/if}
      {#if detail.item.reason}
        <p><strong>Action reason:</strong> {detail.item.reason}</p>
      {/if}
    </section>

    <section class="timeline" aria-labelledby="queue-timeline">
      <h3 id="queue-timeline">Timeline</h3>
      {#if detail.events.length === 0}
        <p class="detail-message">No transitions recorded.</p>
      {:else}
        <ol>
          {#each detail.events as event (event.id)}
            <li>
              <span class="timeline-mark" aria-hidden="true"></span>
              <div>
                <strong>{event.summary}</strong>
                <span>{words(event.kind)} · {words(event.state)}</span>
                <time datetime={event.created_at}>{absolute(event.created_at)}</time>
                <span>Actor {event.actor}</span>
              </div>
            </li>
          {/each}
        </ol>
      {/if}
    </section>
  {/if}

  {#snippet footer()}
    <Button onclick={onClose}>Close</Button>
  {/snippet}
</Modal>

<style>
  .detail-message {
    color: var(--text-muted);
    margin: 0;
  }
  .detail-error,
  .blocking {
    color: var(--danger);
  }
  .facts {
    display: grid;
    gap: 1px;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    margin: 0;
    overflow: hidden;
  }
  .facts div {
    background: var(--control-bg);
    display: grid;
    gap: var(--space-1);
    min-width: 0;
    padding: var(--space-3);
  }
  dt {
    color: var(--text-muted);
    font-size: 0.66rem;
    font-weight: 760;
    letter-spacing: 0.045em;
    text-transform: uppercase;
  }
  dd {
    font-size: 0.78rem;
    margin: 0;
    overflow-wrap: anywhere;
  }
  .workload-detail,
  .timeline {
    border-top: 1px solid var(--border-subtle);
    padding-top: var(--space-4);
  }
  h3 {
    font-size: 0.78rem;
    letter-spacing: 0.04em;
    margin: 0 0 var(--space-3);
    text-transform: uppercase;
  }
  .workload-detail p {
    color: var(--text-muted);
    font-size: 0.78rem;
    line-height: var(--leading-meta);
    margin: var(--space-2) 0 0;
  }
  .timeline ol {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .timeline li {
    display: grid;
    gap: var(--space-3);
    grid-template-columns: 0.7rem minmax(0, 1fr);
    padding-bottom: var(--space-4);
    position: relative;
  }
  .timeline li:not(:last-child)::before {
    background: var(--border-subtle);
    content: '';
    left: 0.3rem;
    position: absolute;
    top: 0.6rem;
    bottom: -0.1rem;
    width: 1px;
  }
  .timeline-mark {
    background: var(--brand-action);
    border: 2px solid var(--dialog-bg);
    border-radius: 50%;
    height: 0.65rem;
    margin-top: 0.2rem;
    position: relative;
    width: 0.65rem;
    z-index: 1;
  }
  .timeline li div,
  .timeline li div > span,
  .timeline time {
    display: grid;
  }
  .timeline strong {
    font-size: 0.78rem;
  }
  .timeline li div > span,
  .timeline time {
    color: var(--text-muted);
    font-size: 0.7rem;
    margin-top: var(--space-1);
  }
  @media (max-width: 28rem) {
    .facts {
      grid-template-columns: 1fr;
    }
  }
</style>

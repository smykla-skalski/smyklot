<script lang="ts">
  import type { PanelApi } from '../lib/api';
  import { formatRelative, formatTimestamp } from '../lib/format';
  import type { SecurityNotification } from '../lib/types';
  import Avatar from './Avatar.svelte';
  import Icon from './Icon.svelte';
  import Modal from './Modal.svelte';

  const {
    fetchPage,
    markRead,
    refreshVersion = 0,
  }: {
    fetchPage: PanelApi['fetchNotifications'];
    markRead: PanelApi['markNotificationRead'];
    refreshVersion?: number;
  } = $props();

  let open = $state(false);
  let trigger = $state<HTMLButtonElement | null>(null);
  let items = $state<SecurityNotification[]>([]);
  let unread = $state(0);
  let total = $state(0);
  let nextCursor = $state<string | null>(null);
  let loading = $state(false);
  let loadingMore = $state(false);
  let failure = $state<string | null>(null);
  let now = $state(Date.now());
  let expandedAuditId = $state<string | null>(null);
  const groups = $derived(groupNotifications(items));

  async function load(reset = true): Promise<void> {
    if (reset ? loading : loadingMore) return;
    if (reset) loading = true;
    else loadingMore = true;
    failure = null;
    try {
      const page = await fetchPage({
        ...(reset || nextCursor === null ? {} : { cursor: nextCursor }),
        limit: 20,
      });
      items = reset ? page.items : merge(items, page.items);
      unread = page.unread;
      total = page.total;
      nextCursor = page.next_cursor;
    } catch (error) {
      failure = error instanceof Error ? error.message : String(error);
    } finally {
      loading = false;
      loadingMore = false;
    }
  }

  function openInbox(): void {
    open = true;
    now = Date.now();
    void load();
  }

  function closeInbox(): void {
    open = false;
    expandedAuditId = null;
    window.setTimeout(() => trigger?.focus(), 0);
  }

  function toggleAuditRecord(event: MouseEvent, notification: SecurityNotification): void {
    event.preventDefault();
    expandedAuditId =
      expandedAuditId === notification.audit_event_id ? null : notification.audit_event_id;
    void read(notification);
  }

  async function read(notification: SecurityNotification): Promise<void> {
    if (notification.read_at !== undefined) return;
    try {
      const updated = await markRead(notification.id);
      items = items.map((item) => (item.id === updated.id ? updated : item));
      unread = Math.max(0, unread - 1);
    } catch (error) {
      failure = error instanceof Error ? error.message : String(error);
    }
  }

  function actionLabel(action: string): string {
    if (action === 'target.settings.updated') return 'Installation settings changed';
    if (action === 'repository.settings.updated') return 'Repository settings changed';
    return action
      .split('.')
      .map((part) => part[0]?.toLocaleUpperCase() + part.slice(1))
      .join(' ');
  }

  function merge(
    current: SecurityNotification[],
    incoming: SecurityNotification[],
  ): SecurityNotification[] {
    const known = new Set(current.map((notification) => notification.id));
    return [...current, ...incoming.filter((notification) => !known.has(notification.id))];
  }

  function groupNotifications(notifications: SecurityNotification[]) {
    const grouped: Array<{ id: string; events: SecurityNotification[] }> = [];
    for (const notification of notifications) {
      const group = grouped.find((entry) => entry.id === notification.elevation_id);
      if (group === undefined) {
        grouped.push({ id: notification.elevation_id, events: [notification] });
      } else {
        group.events.push(notification);
      }
    }
    return grouped;
  }

  $effect(() => {
    if (refreshVersion >= 0) void load();
  });
</script>

<button
  class="account-action notification-trigger"
  type="button"
  bind:this={trigger}
  aria-label={unread === 0 ? 'Security notifications' : `Security notifications, ${unread} unread`}
  onclick={openInbox}
>
  <Icon name="notifications" size={16} />
  <span>Security notifications</span>
  {#if unread > 0}<span class="unread-count" aria-hidden="true">{unread > 99 ? '99+' : unread}</span
    >{/if}
</button>

<Modal
  id="security-notifications"
  {open}
  title="Security notifications"
  description="Audited Root activity for installations you own"
  variant="wide"
  returnFocus={trigger}
  onClose={closeInbox}
>
  <div class="inbox-summary">
    <span class="summary-icon"><Icon name="notifications" size={20} /></span>
    <div>
      <strong
        >{unread === 0
          ? 'You are up to date'
          : `${unread} unread ${unread === 1 ? 'event' : 'events'}`}</strong
      >
      <p>{total} security {total === 1 ? 'event' : 'events'} retained in your inbox</p>
    </div>
    <button class="btn btn-row" type="button" disabled={loading} onclick={() => load()}>
      <Icon name="refresh" size={15} /> Refresh
    </button>
  </div>

  {#if failure !== null}
    <p class="form-error" role="alert">{failure}</p>
  {/if}

  <div class="notification-list" aria-live="polite">
    {#if loading && items.length === 0}
      <p class="inbox-state">Reading security notifications…</p>
    {:else}
      {#each groups as group (group.id)}
        <section class="notification-group" aria-label={`Elevation ${group.id}`}>
          <header>
            <span>Elevation {group.id.slice(-10)}</span>
            <span>{group.events.length} {group.events.length === 1 ? 'event' : 'events'}</span>
            {#if group.events[0]?.reason !== undefined}
              <strong>{group.events[0].reason}</strong>
            {/if}
          </header>
          {#each group.events as notification (notification.id)}
            <article
              id={`audit-event-${notification.audit_event_id}`}
              class:unread={notification.read_at === undefined}
            >
              <Avatar account={notification.actor} size={34} />
              <div class="notification-copy">
                <div class="notification-title">
                  <strong>{actionLabel(notification.action)}</strong>
                  {#if notification.read_at === undefined}<span>New</span>{/if}
                </div>
                <p>
                  <strong>{notification.actor.display_name}</strong> used elevated access on
                  <strong>{notification.installation.display_name}</strong>
                </p>
                <div class="notification-meta">
                  <time
                    datetime={notification.created_at}
                    title={formatTimestamp(notification.created_at)}
                    >{formatRelative(notification.created_at, now)}</time
                  >
                  <a
                    class="audit-link"
                    href={`#audit-event-${notification.audit_event_id}`}
                    aria-expanded={expandedAuditId === notification.audit_event_id}
                    onclick={(event) => toggleAuditRecord(event, notification)}
                    >Audit #{notification.audit_event_id}</a
                  >
                </div>
                {#if expandedAuditId === notification.audit_event_id}
                  <dl class="audit-record">
                    <div>
                      <dt>Event</dt>
                      <dd>#{notification.audit_event_id}</dd>
                    </div>
                    <div>
                      <dt>Action</dt>
                      <dd>{notification.action}</dd>
                    </div>
                    <div>
                      <dt>Installation</dt>
                      <dd>@{notification.installation.login}</dd>
                    </div>
                    <div>
                      <dt>Actor</dt>
                      <dd>@{notification.actor.login}</dd>
                    </div>
                    <div>
                      <dt>Elevation</dt>
                      <dd>{notification.elevation_id}</dd>
                    </div>
                    {#if notification.reason !== undefined}
                      <div class="wide">
                        <dt>Reason</dt>
                        <dd>{notification.reason}</dd>
                      </div>
                    {/if}
                  </dl>
                {/if}
              </div>
              {#if notification.read_at === undefined}
                <button
                  class="btn btn-row mark-read"
                  type="button"
                  aria-label={`Mark ${actionLabel(notification.action)} for ${notification.installation.display_name} as read`}
                  onclick={() => read(notification)}
                >
                  Mark read
                </button>
              {:else}
                <span class="read-state"><Icon name="success" size={16} /> Read</span>
              {/if}
            </article>
          {/each}
        </section>
      {:else}
        <div class="inbox-empty">
          <span><Icon name="success" size={22} /></span>
          <strong>No security events</strong>
          <p>Audited Root writes to installations you own will appear here.</p>
        </div>
      {/each}
    {/if}
  </div>

  {#if nextCursor !== null}
    <button class="btn load-more" type="button" disabled={loadingMore} onclick={() => load(false)}>
      {loadingMore ? 'Loading…' : 'Load earlier events'}
    </button>
  {/if}

  {#snippet footer()}
    <button class="btn" type="button" data-modal-focus onclick={closeInbox}>Close</button>
  {/snippet}
</Modal>

<style>
  .notification-trigger {
    position: relative;
  }

  .notification-trigger > span:nth-child(2) {
    flex: 1;
  }

  .unread-count {
    align-items: center;
    background: var(--stop);
    border-radius: 999px;
    color: #fff;
    display: inline-flex;
    font: 700 var(--font-size-micro) / 1 var(--sans);
    height: 1.25rem;
    justify-content: center;
    min-width: 1.25rem;
    padding-inline: 0.3rem;
  }

  .inbox-summary {
    align-items: center;
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-control);
    display: grid;
    gap: var(--space-3);
    grid-template-columns: auto minmax(0, 1fr) auto;
    padding: var(--space-3);
  }

  .inbox-summary p,
  .inbox-summary strong,
  .notification-copy p,
  .inbox-empty p {
    margin: 0;
  }

  .inbox-summary p {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
    margin-top: var(--space-1);
  }

  .summary-icon,
  .inbox-empty > span {
    align-items: center;
    background: var(--accent-tint);
    border-radius: var(--radius-control);
    color: var(--accent);
    display: inline-flex;
    height: 2.5rem;
    justify-content: center;
    width: 2.5rem;
  }

  .form-error {
    margin: var(--space-3) 0 0;
  }

  .notification-list {
    display: grid;
    gap: var(--space-2);
    margin-top: var(--space-4);
  }

  .notification-group {
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-control);
    display: grid;
    gap: 1px;
    overflow: hidden;
  }

  .notification-group > header {
    align-items: center;
    color: var(--text-muted);
    display: flex;
    flex-wrap: wrap;
    font: 600 var(--font-size-micro) / 1.4 var(--mono);
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
  }

  .notification-group > header > span + span::before {
    content: '·';
    margin-inline-end: var(--space-2);
  }

  .notification-group > header strong {
    color: var(--text-secondary);
    flex-basis: 100%;
    font: 500 var(--font-size-compact) / 1.4 var(--sans);
  }

  article {
    align-items: start;
    background: var(--surface-base);
    border: 0;
    border-top: 1px solid var(--border-subtle);
    display: grid;
    gap: var(--space-3);
    grid-template-columns: auto minmax(0, 1fr) auto;
    padding: var(--space-3);
  }

  article.unread {
    background: color-mix(in srgb, var(--accent) 5%, var(--surface-base));
    border-color: color-mix(in srgb, var(--accent) 28%, var(--border-subtle));
    box-shadow: inset 0.2rem 0 var(--accent);
  }

  .notification-copy {
    min-width: 0;
  }

  .notification-title,
  .notification-meta,
  .read-state {
    align-items: center;
    display: flex;
  }

  .notification-title {
    gap: var(--space-2);
  }

  .notification-title > span {
    background: var(--accent-tint);
    border-radius: var(--radius-control);
    color: var(--accent);
    font-size: var(--font-size-micro);
    font-weight: 750;
    padding: 0.2rem 0.4rem;
    text-transform: uppercase;
  }

  .notification-copy > p {
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
    line-height: 1.5;
    margin-top: var(--space-1);
  }

  .notification-copy > p strong {
    color: var(--text-primary);
    font-weight: 650;
  }

  .notification-meta {
    color: var(--text-muted);
    flex-wrap: wrap;
    font: 500 var(--font-size-micro) / 1.4 var(--mono);
    gap: var(--space-2);
    margin-top: var(--space-2);
  }

  .notification-meta > * + *::before {
    content: '·';
    margin-inline-end: var(--space-2);
  }

  .audit-link {
    color: var(--accent);
    font: inherit;
    text-decoration: underline;
    text-decoration-color: color-mix(in srgb, currentColor 45%, transparent);
    text-underline-offset: 0.15em;
  }

  .audit-link:hover {
    text-decoration-color: currentColor;
  }

  .audit-record {
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-control);
    display: grid;
    gap: var(--space-2) var(--space-3);
    grid-template-columns: repeat(2, minmax(0, 1fr));
    margin: var(--space-3) 0 0;
    padding: var(--space-3);
  }

  .audit-record div {
    min-width: 0;
  }

  .audit-record .wide {
    grid-column: 1 / -1;
  }

  .audit-record dt {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
    text-transform: uppercase;
  }

  .audit-record dd {
    color: var(--text-primary);
    font: 500 var(--font-size-compact) / 1.45 var(--mono);
    margin: var(--space-1) 0 0;
    overflow-wrap: anywhere;
  }

  .read-state {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    gap: var(--space-1);
    padding: var(--space-2);
  }

  .inbox-state,
  .inbox-empty {
    color: var(--text-secondary);
    margin: 0;
    min-height: 10rem;
    padding: var(--space-6);
    text-align: center;
  }

  .inbox-empty {
    align-items: center;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    justify-content: center;
  }

  .inbox-empty p {
    font-size: var(--font-size-compact);
  }

  .load-more {
    margin: var(--space-3) auto 0;
  }

  @media (max-width: 38rem) {
    .inbox-summary,
    article {
      grid-template-columns: auto minmax(0, 1fr);
    }

    .inbox-summary .btn,
    article > :last-child {
      grid-column: 1 / -1;
      justify-self: start;
    }

    .audit-record {
      grid-template-columns: 1fr;
    }

    .audit-record .wide {
      grid-column: auto;
    }
  }
</style>

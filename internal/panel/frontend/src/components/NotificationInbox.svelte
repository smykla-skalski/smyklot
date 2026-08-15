<script lang="ts">
  import { untrack } from 'svelte';
  import type { PanelApi } from '../lib/api';
  import { dialogRoute } from '../lib/dialog-route.svelte';
  import { formatRelative, formatTimestamp } from '../lib/format';
  import type { SecurityNotification } from '../lib/types';
  import Chip from './Chip.svelte';
  import Icon from './Icon.svelte';
  import Modal from './Modal.svelte';

  const {
    fetchPage,
    markRead,
    refreshVersion = 0,
    onUnread,
    onOpen,
  }: {
    fetchPage: PanelApi['fetchNotifications'];
    markRead: PanelApi['markNotificationRead'];
    refreshVersion?: number;
    /** Reports the unread count so the host can badge its own trigger. */
    onUnread?: (unread: number) => void;
    /** The trigger lives in a menu, which has no reason to stay open behind the dialog. */
    onOpen?: () => void;
  } = $props();

  /** Names the dialog in the address, and is the `id` the dialog carries. */
  const INBOX_DIALOG = 'security-notifications';

  /* Whatever the address names, so a reload keeps the inbox open and a link to it
     can be sent to somebody who needs to read the same thing. */
  const open = $derived(dialogRoute.isOpen(INBOX_DIALOG));
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
    dialogRoute.open(INBOX_DIALOG);
    now = Date.now();
    onOpen?.();
  }

  /** Lets a caller outside the account menu open the inbox - the overview's
   *  unread-security-events card is about exactly these notifications. */
  export function showInbox(): void {
    openInbox();
  }

  function closeInbox(): void {
    if (dialogRoute.isOpen(INBOX_DIALOG)) dialogRoute.close();
    window.setTimeout(() => trigger?.focus(), 0);
  }

  /* The list is loaded because the dialog is open, not because a button was
     pressed, so an address that arrives with the inbox already open fills it.
     Collapsing the expanded record on the way out belongs here for the same
     reason: Back closes the dialog without passing through any handler. */
  $effect(() => {
    if (!open) {
      untrack(() => (expandedAuditId = null));
      return;
    }
    untrack(() => void load());
  });

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
    if (action === 'target.settings.updated') return 'Workspace settings changed';
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

  // Loads eagerly on purpose: the sidebar badge reads `unread` from this same
  // fetch, so the list load cannot wait for the dialog to open.
  $effect(() => {
    if (refreshVersion >= 0) void load();
  });

  $effect(() => {
    onUnread?.(unread);
  });
</script>

<button
  class="notification-trigger"
  type="button"
  bind:this={trigger}
  aria-label={unread === 0 ? 'Inbox' : `Inbox, ${unread} unread`}
  onclick={openInbox}
>
  <span class="trigger-icon"><Icon name="notifications" size={16} /></span>
  <span class="trigger-text">Inbox</span>
  {#if unread > 0}<span class="unread-count" aria-hidden="true">{unread > 99 ? '99+' : unread}</span
    >{/if}
</button>

<Modal
  id={INBOX_DIALOG}
  {open}
  title="Inbox"
  description="Audited Root activity on workspaces you own"
  variant="wide"
  returnFocus={trigger}
  onClose={closeInbox}
>
  {#snippet headerExtra()}
    <Chip tone="accent">{unread === 0 ? 'All read' : `${unread} unread`} · {total} retained</Chip>
  {/snippet}

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
            <span class="group-id">Elevation {group.id.slice(-10)}</span>
            <span class="group-reason">{group.events[0]?.reason ?? ''}</span>
            <span class="group-count"
              >{group.events.length} {group.events.length === 1 ? 'event' : 'events'}</span
            >
          </header>
          {#each group.events as notification (notification.id)}
            <article
              id={`audit-event-${notification.audit_event_id}`}
              class:unread={notification.read_at === undefined}
            >
              <span class="unread-slot" aria-hidden="true"></span>
              <div class="notification-copy">
                <div class="notification-title">{actionLabel(notification.action)}</div>
                <p>
                  {notification.actor.display_name} used elevated access on
                  {notification.installation.display_name}
                </p>
                <div class="notification-meta">
                  <time
                    datetime={notification.created_at}
                    title={formatTimestamp(notification.created_at)}
                    >{formatRelative(notification.created_at, now)}</time
                  ><a
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
                  class="btn mark-read"
                  type="button"
                  aria-label={`Mark ${actionLabel(notification.action)} for ${notification.installation.display_name} as read`}
                  onclick={() => read(notification)}
                >
                  Mark read
                </button>
              {:else}
                <span class="read-state"
                  ><span class="read-slot"><Icon name="check" size={14} /></span><span
                    class="cap-trim">Read</span
                  ></span
                >
              {/if}
            </article>
          {/each}
        </section>
      {:else}
        <div class="inbox-empty">
          <span><Icon name="success" size={22} /></span>
          <strong>No security events</strong>
          <p>Audited Root writes to workspaces you own will appear here</p>
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
    <button class="btn btn-ghost" type="button" data-modal-focus onclick={closeInbox}>Close</button>
  {/snippet}
</Modal>

<style>
  /* Styled here rather than by the host menu: a class shared across components
     silently loses its styles to Svelte's scoping (the trigger once rendered
     with UA-default gray because of exactly that). */
  .notification-trigger {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: calc(var(--radius-popover) - 6px);
    color: var(--sidebar-menu-text);
    cursor: pointer;
    display: flex;
    font: 500 var(--font-size-meta) / 1 var(--sans);
    gap: 0.625rem;
    min-height: 2.5rem;
    padding: 0 10px;
    text-align: left;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      transform var(--duration-press) var(--ease-standard);
    width: 100%;
  }

  .notification-trigger:hover {
    background: var(--sidebar-menu-hover);
  }

  .notification-trigger:active {
    background: var(--sidebar-menu-pressed);
  }

  .trigger-icon {
    color: var(--sidebar-menu-muted);
    display: grid;
    flex: none;
    place-items: center;
    width: 1.125rem;
  }

  .trigger-text {
    flex: 1;
    text-box: trim-both cap alphabetic;
  }

  .unread-count {
    align-items: center;
    background: var(--unread-badge-bg);
    border-radius: 999px;
    color: var(--unread-badge-text);
    display: inline-flex;
    font: 700 var(--font-size-micro) / 1 var(--sans);
    height: 1.25rem;
    justify-content: center;
    min-width: 1.25rem;
    padding-inline: 0.3rem;
  }

  .notification-copy p,
  .inbox-empty p {
    margin: 0;
  }

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
    gap: var(--space-3);
    margin-top: 0;
  }

  /* No grid gap: each item already draws its own top rule, so a 1px gap showing
     the group's inset behind it doubled the divider and made the group 2px
     taller than the approved one. */
  .notification-group {
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-control);
    display: grid;
    overflow: hidden;
  }

  /* Flex, not a fixed three-track grid: on the grid the middle track took all
     the slack and ellipsised a reason that fits, while the count sat away from
     the edge. Here the reason is only clipped when it genuinely overruns. */
  .notification-group > header {
    align-items: baseline;
    display: flex;
    gap: 0.6rem;
    padding: 0.6rem var(--space-3) 0.55rem;
  }

  .notification-group > header .group-count {
    margin-left: auto;
    white-space: nowrap;
  }

  .notification-group > header .group-reason {
    min-width: 0;
  }

  .group-id,
  .group-count {
    color: var(--text-muted);
    font: 600 var(--font-size-micro) / 1 var(--mono);
  }

  .group-id {
    color: var(--text-secondary);
  }

  .group-reason {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
    line-height: 1.35;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
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
    background: color-mix(in srgb, var(--accent) 4%, var(--surface-base));
  }

  /* Fixed-width in every item so read and unread text columns align; the dot
     rides the title line of unread items only. */
  .unread-slot {
    padding-top: 0.265rem;
    width: 0.5rem;
  }

  article.unread .unread-slot::before {
    background: var(--accent);
    border-radius: 50%;
    content: '';
    display: block;
    height: 0.5rem;
    width: 0.5rem;
  }

  .notification-copy {
    min-width: 0;
  }

  .read-state {
    align-items: center;
    display: flex;
  }

  .notification-title {
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
    font-weight: 700;
    line-height: 1.3;
  }

  article.unread .notification-title {
    color: var(--text-primary);
    font-weight: 700;
  }

  .notification-copy > p {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
    line-height: 1.45;
    margin-top: 0.3rem;
  }

  .notification-meta {
    color: var(--text-muted);
    display: block;
    font: 500 var(--font-size-micro) / 1.4 var(--mono);
    margin-top: 0.3rem;
  }

  /* The separator belongs to the meta line, not to the link: non-breaking
     spaces so it keeps the mono advance the approved line measures, and muted
     so the dot does not read as part of the link's label. */
  .audit-link::before {
    color: var(--text-muted);
    content: '\00a0\00b7\00a0';
  }

  .audit-link {
    color: var(--brand-action-text);
    font: inherit;
    text-decoration: none;
  }

  .audit-link:hover {
    text-decoration: underline;
    text-underline-offset: 0.15em;
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

  .mark-read,
  .read-state {
    align-self: center;
  }

  .read-state {
    color: var(--text-muted);
    font: 600 var(--font-size-compact) / 1.5 var(--sans);
    gap: var(--space-2);
  }

  .read-slot {
    display: grid;
    flex: none;
    height: 1.125rem;
    place-items: center;
    width: 1.125rem;
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
    article {
      grid-template-columns: auto minmax(0, 1fr);
    }

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

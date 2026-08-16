<script lang="ts">
  import { untrack } from 'svelte';

  import type { PanelApi } from '../lib/api';
  import { formatRelative, formatTimestamp } from '../lib/format';
  import { LatestRequest } from '../lib/latest-request';
  import type { SecurityNotification } from '../lib/types';
  import Chip from './Chip.svelte';
  import Icon from './Icon.svelte';
  import PanelHeader from './PanelHeader.svelte';
  import ResultProblem from './ResultProblem.svelte';

  const {
    fetchPage,
    markRead,
    refreshVersion = 0,
    onUnread,
  }: {
    fetchPage: PanelApi['fetchNotifications'];
    markRead: PanelApi['markNotificationRead'];
    refreshVersion?: number;
    /** Reports the count back, so the sidebar's badge answers to what was read here. */
    onUnread?: (unread: number) => void;
  } = $props();

  const PAGE_SIZE = 20;

  let items = $state<SecurityNotification[]>([]);
  let unread = $state(0);
  let total = $state(0);
  let nextCursor = $state<string | null>(null);
  let loading = $state(false);
  let loadingMore = $state(false);
  let problem = $state<string | null>(null);
  let loaded = $state(false);
  let now = $state(0);
  let expandedAuditId = $state<string | null>(null);
  const groups = $derived(groupNotifications(items));
  const reads = new LatestRequest();
  /**
   * The count is guarded apart from the list, because the two are answered by
   * different requests.
   *
   * A read of the list carries a count with it, and a read of the count carries
   * no list. Whichever was asked for last is the one to believe, and that is not
   * the same question as which list is on screen.
   */
  const counts = new LatestRequest();

  async function load(reset = true): Promise<void> {
    if (reset ? loading : loadingMore) return;
    /* A read of the whole list and a read of the next page can be in the air at
       once - the stream refreshes while somebody is pressing for earlier events -
       and only the newer one may commit. Without this the refresh could land
       after the page it was racing and take the earlier events away again,
       leaving a reader who pressed the button watching it undo itself. */
    const read = reads.begin();
    const count = counts.begin();
    if (reset) loading = true;
    else loadingMore = true;
    problem = null;
    try {
      const page = await fetchPage({
        ...(reset || nextCursor === null ? {} : { cursor: nextCursor }),
        limit: PAGE_SIZE,
      });
      if (counts.isCurrent(count)) {
        unread = page.unread;
        total = page.total;
      }
      if (!reads.isCurrent(read)) return;
      items = reset ? page.items : merge(items, page.items);
      nextCursor = page.next_cursor;
      loaded = true;
      /* Stamped from the answer rather than at mount: every relative time on the
         page is measured against the same instant, and a reader who leaves the
         tab open does not come back to a list that says "now" about yesterday. */
      now = Date.now();
    } catch (error) {
      if (reads.isCurrent(read)) problem = error instanceof Error ? error.message : String(error);
    } finally {
      // Its own flag, whether or not it was still the newest: the other read owns
      // the other flag, and a control left disabled never comes back.
      if (reset) loading = false;
      else loadingMore = false;
    }
  }

  /* A disclosure, so the control is a button. It was an anchor whose href named
     the article it sat inside, which made every modified click a promise the
     page could not keep: Cmd-click opened a second tab scrolled to the same
     place with nothing expanded, and a plain click had to swallow the address
     to stop it going anywhere. */
  function toggleAuditRecord(notification: SecurityNotification): void {
    expandedAuditId =
      expandedAuditId === notification.audit_event_id ? null : notification.audit_event_id;
    void read(notification);
  }

  async function read(notification: SecurityNotification): Promise<void> {
    if (notification.read_at !== undefined) return;
    try {
      const updated = await markRead(notification.id);
      items = items.map((item) => (item.id === updated.id ? updated : item));
      /* One fewer, straight away. The reader just did that and should see it
         happen, so the count answers the press rather than the round trip after
         it. It can be out by one either way depending on what else was in the
         air, which is what the read below settles. */
      unread = Math.max(0, unread - 1);
      await refreshCount();
    } catch (error) {
      problem = error instanceof Error ? error.message : String(error);
    }
  }

  /**
   * Asks what the count is now, because the subtraction above is a guess.
   *
   * A client cannot tell which way the guess is wrong: a list that arrives while
   * the read is in flight may have been counted by the server with the read
   * already in it, or from a moment before it, and the two are indistinguishable
   * from here. Subtracting anyway leaves the total one below the truth in the
   * first case; refusing to subtract leaves it one above in the second. Neither
   * corrected itself until some other read happened to run.
   *
   * So the guess stands only until the answer arrives, which is one small request
   * per press of a button somebody deliberately pressed.
   */
  async function refreshCount(): Promise<void> {
    const count = counts.begin();
    try {
      const page = await fetchPage({ limit: 1 });
      if (!counts.isCurrent(count)) return;
      unread = page.unread;
      total = page.total;
    } catch {
      /* A count is not worth a failure of its own over a list that is fine. The
         next read of either settles it. */
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

  /**
   * Reads on mount, and again whenever the stream says something changed.
   *
   * `untrack` because `load` reads `loading` before its first await, to refuse a
   * second read while one is in flight. That read is inside this effect, so the
   * effect depended on a value `load` also writes: finishing a read set `loading`
   * back to false, which scheduled another read, which finished. The page asked
   * the server about 1600 times a second, and a failed read never reached the
   * screen because the next attempt cleared it before it could render.
   */
  $effect(() => {
    if (refreshVersion >= 0) untrack(() => void load());
  });

  /* Only once there is a count to report. `unread` starts at zero because nothing
     has been read yet, not because everything has been read, and reporting that
     took the number off the sidebar row for as long as the page took to load -
     ending on the same number it started with, which reads as a flicker. */
  $effect(() => {
    if (loaded) onUnread?.(unread);
  });
</script>

<section class="inbox-page" aria-labelledby="inbox-heading">
  <PanelHeader
    id="inbox-heading"
    title="Inbox"
    description="Audited Root activity on workspaces you own"
  >
    {#snippet actions()}
      <Chip tone="accent">{unread === 0 ? 'All read' : `${unread} unread`} · {total} retained</Chip>
    {/snippet}
  </PanelHeader>

  <div class="inbox-results" aria-busy={loading}>
    <!-- A refresh that failed over a list already read has not made the list wrong. -->
    {#if problem !== null && loaded}
      <ResultProblem
        title="The inbox could not be read"
        {problem}
        busy={loading}
        onRetry={() => void load()}
        overContent
      />
    {/if}

    {#if problem !== null && !loaded}
      <div class="plate inbox-card">
        <ResultProblem
          title="The inbox could not be read"
          {problem}
          busy={loading}
          onRetry={() => void load()}
        />
      </div>
    {:else if loading && !loaded}
      <div class="inbox-skeleton" aria-hidden="true">
        {#each [0, 1, 2, 3] as index (index)}
          <span></span>
        {/each}
      </div>
      <p class="visually-hidden" role="status">Reading security notifications</p>
    {:else}
      <div class="notification-list" aria-live="polite">
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
              <article class:unread={notification.read_at === undefined}>
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
                    ><button
                      type="button"
                      class="audit-toggle"
                      aria-expanded={expandedAuditId === notification.audit_event_id}
                      onclick={() => toggleAuditRecord(notification)}
                      >Audit #{notification.audit_event_id}</button
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
          <div class="plate inbox-card inbox-empty">
            <span><Icon name="success" size={22} /></span>
            <strong>No security events</strong>
            <p>Audited Root writes to workspaces you own will appear here</p>
          </div>
        {/each}
      </div>

      {#if nextCursor !== null}
        <button
          class="btn load-more"
          type="button"
          disabled={loadingMore}
          onclick={() => load(false)}
        >
          {loadingMore ? 'Loading…' : 'Load earlier events'}
        </button>
      {/if}
    {/if}
  </div>
</section>

<style>
  /* A feed, not a table: it scrolls with the page, so the header travels with it
     and the load-more button is reached by reading to the end rather than by
     scrolling a pane inside a fixed frame. Each group is its own card on the
     canvas, the shape a settings page uses, so there is no outer card drawing a
     second border around the cards inside it. */
  .inbox-page,
  .inbox-results {
    display: flex;
    flex-direction: column;
  }

  /* What a group would have been, for the states that have no groups to show. */
  .inbox-card {
    background: var(--surface-base);
    margin-bottom: 0;
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

  .notification-list {
    display: grid;
    gap: var(--space-3);
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
     the edge. Here the reason is only clipped when it genuinely overruns.

     `min-width: 0` because this is a grid item, and a grid item's automatic
     minimum size is its min-content width: without it the header refused to
     shrink to the card, and the group clipped what stuck out. On a 390px window
     that put the event count 69px past the edge, off screen entirely, while the
     reason it was making room for was not ellipsised at all. */
  .notification-group > header {
    align-items: baseline;
    display: flex;
    gap: 0.6rem;
    min-width: 0;
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

  /* An identifier is one word however narrow the window gets: broken across two
     lines it stops being something a reader can match against the audit trail. */
  .group-id {
    color: var(--text-secondary);
    flex: none;
    white-space: nowrap;
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
  .audit-toggle::before {
    color: var(--text-muted);
    content: '\00a0\00b7\00a0';
  }

  .audit-toggle {
    background: none;
    border: 0;
    color: var(--brand-action-text);
    font: inherit;
    padding: 0;
    text-decoration: none;
  }

  .audit-toggle:hover {
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

  .inbox-empty {
    align-items: center;
    color: var(--text-secondary);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    justify-content: center;
    margin: 0;
    min-height: 10rem;
    padding: var(--space-6);
    text-align: center;
  }

  .inbox-empty p {
    font-size: var(--font-size-compact);
  }

  /* One block per group rather than per row: what is arriving is groups, and a
     column of identical row-height bars promised a shape the answer rarely has. */
  .inbox-skeleton {
    display: grid;
    gap: var(--space-3);
  }

  .inbox-skeleton span {
    animation: inbox-skeleton-pulse 1.35s ease-in-out infinite alternate;
    background: var(--surface-inset);
    border-radius: var(--radius-control);
    display: block;
    height: 7.5rem;
  }

  .load-more {
    margin: var(--space-4) auto 0;
  }

  @keyframes inbox-skeleton-pulse {
    from {
      opacity: 0.52;
    }

    to {
      opacity: 0.9;
    }
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

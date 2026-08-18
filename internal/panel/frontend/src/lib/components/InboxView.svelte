<script lang="ts">
  import {
    createInfiniteQuery,
    createMutation,
    useQueryClient,
    type InfiniteData,
  } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';

  import type { PanelApi } from '../api';
  import { formatRelative, formatTimestamp } from '../format';
  import type { NotificationPage, SecurityNotification } from '../types';
  import Button from './Button.svelte';
  import Chip from './Chip.svelte';
  import Icon from './Icon.svelte';
  import PageHeader from './PageHeader.svelte';
  import ResultProblem from './ResultProblem.svelte';

  const {
    fetchPage,
    markRead,
    onUnread,
  }: {
    fetchPage: PanelApi['fetchNotifications'];
    markRead: PanelApi['markNotificationRead'];
    /** Reports the count back, so the sidebar's badge answers to what was read here. */
    onUnread?: (unread: number) => void;
  } = $props();

  const PAGE_SIZE = 20;
  const queryClient = useQueryClient();
  const notificationsQuery = createInfiniteQuery(() => ({
    queryKey: ['notifications'],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      fetchPage({
        ...(pageParam === undefined ? {} : { cursor: pageParam }),
        limit: PAGE_SIZE,
      }),
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
  }));
  const markReadMutation = createMutation(() => ({
    mutationFn: (notificationId: string) => markRead(notificationId),
    onSuccess: (updated) => {
      queryClient.setQueryData<InfiniteData<NotificationPage>>(['notifications'], (current) =>
        current === undefined
          ? current
          : {
              ...current,
              pages: current.pages.map((page) => ({
                ...page,
                items: page.items.map((item) => (item.id === updated.id ? updated : item)),
                unread: Math.max(0, page.unread - 1),
              })),
            },
      );
    },
  }));
  const pages = $derived(notificationsQuery.data?.pages ?? []);
  const items = $derived(mergePages(pages));
  const unread = $derived(pages[0]?.unread ?? 0);
  const total = $derived(pages[0]?.total ?? 0);
  const loading = $derived(notificationsQuery.isFetching && !notificationsQuery.isFetchingNextPage);
  const loadingMore = $derived(notificationsQuery.isFetchingNextPage);
  let actionProblem = $state<string | null>(null);
  const problem = $derived(
    actionProblem ??
      (notificationsQuery.error === null
        ? null
        : notificationsQuery.error instanceof Error
          ? notificationsQuery.error.message
          : String(notificationsQuery.error)),
  );
  const loaded = $derived(notificationsQuery.data !== undefined);
  let now = $state(0);
  let expandedAuditId = $state<string | null>(null);
  const groups = $derived(groupNotifications(items));

  async function load(reset = true): Promise<void> {
    if (reset) await notificationsQuery.refetch();
    else if (notificationsQuery.hasNextPage) await notificationsQuery.fetchNextPage();
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
    actionProblem = null;
    try {
      await markReadMutation.mutateAsync(notification.id);
      await notificationsQuery.refetch();
    } catch (error) {
      actionProblem = error instanceof Error ? error.message : String(error);
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

  function mergePages(notificationPages: NotificationPage[]): SecurityNotification[] {
    const known = new SvelteSet<string>();
    return notificationPages.flatMap((page) =>
      page.items.filter((notification) => {
        if (known.has(notification.id)) return false;
        known.add(notification.id);
        return true;
      }),
    );
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
    if (notificationsQuery.dataUpdatedAt > 0) now = notificationsQuery.dataUpdatedAt;
  });

  /* Only once there is a count to report. `unread` starts at zero because nothing
     has been read yet, not because everything has been read, and reporting that
     took the number off the sidebar row for as long as the page took to load -
     ending on the same number it started with, which reads as a flicker. */
  $effect(() => {
    const count = unread;
    if (loaded) untrack(() => onUnread?.(count));
  });
</script>

<section class="inbox-page" aria-labelledby="inbox-heading">
  <PageHeader
    id="inbox-heading"
    title="Inbox"
    description="Audited Root activity on workspaces you own"
  >
    {#snippet actions()}
      <Chip tone="accent">{unread === 0 ? 'All read' : `${unread} unread`} · {total} retained</Chip>
    {/snippet}
  </PageHeader>

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
                  <Button
                    class="mark-read"
                    aria-label={`Mark ${actionLabel(notification.action)} for ${notification.installation.display_name} as read`}
                    onclick={() => read(notification)}
                  >
                    Mark read
                  </Button>
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

      {#if notificationsQuery.hasNextPage}
        <Button class="load-more" disabled={loadingMore} onclick={() => load(false)}>
          {loadingMore ? 'Loading…' : 'Load earlier events'}
        </Button>
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

  /* `:global` because `Button` renders the control, so it wears that component's
     scope class and a bare `.mark-read` no longer matches. Anchored to the list, so
     it still reaches nothing outside this component. Load-bearing: without it the
     control loses its vertical centring against the event beside it. */
  .notification-list :global(.mark-read),
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
    /* The panel's one placeholder pulse, from `app.css`. The endpoints stay this
       placeholder's own - the eight copies of this keyframe had drifted into three
       different ranges, and collapsing them was not allowed to move any of them. */
    --skeleton-from: 0.52;
    --skeleton-to: 0.9;
    animation: skeleton-pulse 1.35s ease-in-out infinite alternate;
    background: var(--surface-inset);
    border-radius: var(--radius-control);
    display: block;
    height: 7.5rem;
  }

  .inbox-results :global(.load-more) {
    margin: var(--space-4) auto 0;
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

    /* The reason is the only part of this header a reader has not already got
       from the elevation id, and sharing the line with both of the others left
       it 121px: "Restore command handling during production i…". It takes the
       line below instead, where it can say the whole thing. */
    .notification-group > header {
      flex-wrap: wrap;
    }

    .notification-group > header .group-reason {
      flex-basis: 100%;
      white-space: normal;
    }
  }
</style>

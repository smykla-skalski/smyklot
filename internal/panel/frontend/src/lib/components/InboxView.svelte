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
  import type { NotificationPage, SecurityNotification } from '../types';
  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import Chip from './Chip.svelte';
  import PageHeader from './PageHeader.svelte';
  import Pill from './Pill.svelte';
  import RelativeTime from './RelativeTime.svelte';
  import ResultProblem from './ResultProblem.svelte';

  const {
    fetchPage,
    markRead,
    markAllRead,
    onUnread,
    viewerName = 'Personal',
    auditHref,
  }: {
    fetchPage: PanelApi['fetchNotifications'];
    markRead: PanelApi['markNotificationRead'];
    markAllRead?: PanelApi['markAllNotificationsRead'];
    /** Reports the count back, so the sidebar's badge answers to what was read here. */
    onUnread?: (unread: number) => void;
    /** Whose inbox it is, which is what the page's eyebrow says. */
    viewerName?: string;
    /** Where one receipt's audit entry lives, when the reader can reach that workspace. */
    auditHref?: (notification: SecurityNotification) => string | undefined;
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
  const groups = $derived(groupNotifications(items));

  async function load(reset = true): Promise<void> {
    if (reset) await notificationsQuery.refetch();
    else if (notificationsQuery.hasNextPage) await notificationsQuery.fetchNextPage();
  }

  let clearing = $state(false);

  /**
   * Every unread one at once, in one request.
   *
   * A loop over the loaded pages would be a lie the moment the list has a page the
   * reader has not reached: the button says all, so the server empties all.
   */
  async function readEverything(): Promise<void> {
    if (markAllRead === undefined || clearing) return;
    clearing = true;
    actionProblem = null;
    try {
      await markAllRead();
      await notificationsQuery.refetch();
    } catch (error) {
      actionProblem = error instanceof Error ? error.message : String(error);
    } finally {
      clearing = false;
    }
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

  /**
   * The wire's word for a scope, said the way the reader's dictionary says it.
   *
   * A notification is raised for every audit action an operator's visit writes, and that
   * set is open - so the words are translated rather than the actions listed. Four titles
   * used to be spelled out here and everything else fell through to the wire, which is how
   * the inbox came to announce "Installation settings saved" and would have announced
   * "Elevation started" the moment either action reached it.
   *
   * `installation` is a KEY here, not a word: it is what the stored audit action says,
   * and rows already written say it. The value beside it is what a reader is shown.
   */
  const SCOPE_WORDS: Record<string, string> = {
    installation: 'workspace',
    target: 'workspace',
    elevation: 'operator visit',
    runtime: 'service',
  };

  function actionLabel(action: string): string {
    const said = action
      .split('.')
      .map((part) => SCOPE_WORDS[part] ?? part.replaceAll('_', ' '))
      .join(' ');

    return said.charAt(0).toLocaleUpperCase() + said.slice(1);
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

<!--
@component
What has happened that concerns you personally, rather than anything about a workspace.
It is the panel's one personal page, which is why it lives at its own address rather
than inside a workspace.

It reports its unread count back through `onUnread`, so the rail's badge answers to
what was actually read here rather than keeping a second tally that can disagree.

Pages arrive on a cursor as the reader reaches the end, so there is no total and no
pager - a notification list has no last page worth naming.
-->

<section class="inbox-page" aria-labelledby="inbox-heading">
  <PageHeader
    id="inbox-heading"
    eyebrow={viewerName}
    title="Inbox"
    description="When an operator touches a workspace you own, the receipt lands here"
  >
    {#snippet actions()}
      <!-- The unread count is the only number the head owes a reader. "Retained" was a
           second one that answered nothing they can act on - the list itself says how
           much there is, and there is a control for the one thing they want to do. -->
      {#if unread > 0}
        <Chip tone="accent">{unread} unread</Chip>
        <Button tone="quiet" disabled={clearing} onclick={() => void readEverything()}>
          {clearing ? 'Marking…' : 'Mark all read'}
        </Button>
      {:else}
        <Chip tone="clear">All read</Chip>
      {/if}
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
      <!-- ONE ELEVATION IS ONE CARD, AND THE REASON IS ITS TITLE. What a reader wants to
           know is why somebody with operator access was in their workspace; the session
           it happened under is bookkeeping, and it lives on the audit entry each row
           links to rather than at the head of the card that opens with the reason. -->
      <div class="notification-list card-stack" aria-live="polite">
        {#each groups as group (group.id)}
          <Card label={group.events[0]?.reason ?? 'Operator visit'}>
            <div class="card-head">
              <h2 class="card-title">{group.events[0]?.reason ?? 'Operator visit'}</h2>
              <span class="card-meta"
                >{group.events[0]?.workspace.display_name ?? ''} · {group.events.length}
                {group.events.length === 1 ? 'event' : 'events'}</span
              >
            </div>
            <div class="object-list">
              {#each group.events as notification (notification.id)}
                <div class="object-row">
                  <span class="object-main">
                    <span class="object-name-row">
                      <span class="object-name">{actionLabel(notification.action)}</span>
                      {#if notification.read_at === undefined}
                        <Pill tone="warning">Unread</Pill>
                      {/if}
                    </span>
                    <span class="object-sum"
                      >{notification.actor.display_name}, as operator ·
                      <RelativeTime value={notification.created_at} nowMs={now} />
                      ·
                      {#if auditHref !== undefined}
                        <a href={auditHref(notification)}
                          >audit entry in {notification.workspace.display_name}</a
                        >
                      {:else}
                        audit entry #{notification.audit_event_id}
                      {/if}
                      {#if notification.read_at !== undefined}· read{/if}</span
                    >
                  </span>
                  <span class="object-side">
                    {#if notification.read_at === undefined}
                      <Button
                        tone="quiet"
                        aria-label={`Mark read - ${actionLabel(notification.action)} for ${notification.workspace.display_name}`}
                        onclick={() => read(notification)}
                      >
                        Mark read
                      </Button>
                    {/if}
                  </span>
                </div>
              {/each}
            </div>
          </Card>
        {:else}
          <Card>
            <div class="state-panel">
              <span
                ><strong>Nothing has needed your attention.</strong> When an operator writes to a workspace
                you own, the receipt lands here</span
              >
            </div>
          </Card>
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
    animation: skeleton-pulse var(--rhythm-shimmer) var(--ease-inout) infinite alternate;
    background: var(--surface-inset);
    border-radius: var(--radius-control);
    display: block;
    height: 7.5rem;
  }

  .inbox-results :global(.load-more) {
    margin: var(--space-4) auto 0;
  }

  /* No phone block. The row is an `.object-row` and the card head is a `.card-head`,
     and both already know what to do at every width - which is the point of using
     them rather than a shape only this page draws. */
</style>

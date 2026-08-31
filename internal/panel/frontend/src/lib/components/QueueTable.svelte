<script lang="ts">
  import type { QueueActionType, QueueItem } from '#lib/types.js';
  import { cubicOut } from 'svelte/easing';
  import { onMount } from 'svelte';
  import { flip } from 'svelte/animate';
  import { MediaQuery } from 'svelte/reactivity';
  import { fade } from 'svelte/transition';
  import ActionMenu, { type ActionMenuItem } from './ActionMenu.svelte';
  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import Pill, { type PillTone } from './Pill.svelte';

  const {
    items,
    clock = Date.now,
    groupTitle,
    onOpen,
    onAction,
  }: {
    items: readonly QueueItem[];
    clock?: () => number;
    /**
     * What to call the one group a narrowed view leaves.
     *
     * The groups answer a reader looking at everything. A reader who has asked for the
     * running work is told what it is by the control they pressed, and a card over it
     * headed "Running and waiting" answers a question they have already narrowed.
     */
    groupTitle?: string;
    onOpen: (item: QueueItem) => void;
    onAction: (item: QueueItem, action: QueueActionType) => void;
  } = $props();

  type QueueMenuAction = QueueActionType | 'details';
  const reducedMotion = new MediaQuery('prefers-reduced-motion: reduce');
  let motionEnabled = $state(false);
  const still = $derived(reducedMotion.current || !motionEnabled);
  const rowMotion = $derived({ duration: still ? 0 : 150, easing: cubicOut });
  const rowArriving = $derived({ duration: still ? 0 : 120, delay: still ? 0 : 15 });
  const rowLeaving = $derived({ duration: still ? 0 : 70 });
  /* A row's standing can change while somebody is reading it - another operator raises a
     priority, the service starts the work - and a value that swaps with no motion at all
     reads as text that was always there. Same swap as the console's own queue panel. */
  const valueMotion = $derived({ duration: still ? 0 : 90 });

  onMount(() => {
    const frame = window.requestAnimationFrame(() => (motionEnabled = true));
    return () => window.cancelAnimationFrame(frame);
  });

  /**
   * The three questions a reader has of a queue, in the order they have them.
   *
   * A decision is theirs to make and nothing moves until they make it, so it leads. Then
   * what the service is doing on its own, and last what it already did. The states
   * inside each group are the service's vocabulary, which nothing outside this map has
   * to know.
   */
  const GROUPS = [
    {
      id: 'decision',
      title: 'Needs a decision',
      states: ['awaiting_approval'],
    },
    {
      id: 'live',
      title: 'Running and waiting',
      states: ['running', 'ready', 'scheduled', 'blocked', 'retrying'],
    },
    {
      id: 'done',
      title: 'Done',
      states: ['succeeded', 'failed', 'cancelled', 'superseded'],
    },
  ] as const satisfies ReadonlyArray<{
    id: string;
    title: string;
    states: ReadonlyArray<QueueItem['state']>;
  }>;

  const grouped = $derived(
    GROUPS.map((group) => ({
      ...group,
      items: items.filter((item) => (group.states as readonly string[]).includes(item.state)),
    })).filter((group) => group.items.length > 0),
  );

  function words(value: string): string {
    return value.replaceAll('_', ' ').replace(/^./, (letter) => letter.toUpperCase());
  }

  function absolute(value: string, timeZone?: string): string {
    return new Intl.DateTimeFormat(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: 'numeric',
      minute: '2-digit',
      timeZoneName: 'short',
      ...(timeZone === undefined ? {} : { timeZone }),
    }).format(new Date(value));
  }

  function countdown(value: string): string {
    const seconds = Math.round((new Date(value).getTime() - clock()) / 1000);
    if (seconds <= 0) return 'now';
    if (seconds < 60) return `in ${seconds} seconds`;
    if (seconds < 3600) return `in ${Math.ceil(seconds / 60)} minutes`;
    if (seconds < 86_400) return `in ${Math.ceil(seconds / 3600)} hours`;
    return `in ${Math.ceil(seconds / 86_400)} days`;
  }

  function ago(value: string): string {
    const seconds = Math.round((clock() - new Date(value).getTime()) / 1000);
    if (seconds < 60) return 'just now';
    if (seconds < 3600) return `${Math.floor(seconds / 60)} minutes ago`;
    if (seconds < 86_400) return `${Math.floor(seconds / 3600)} hours ago`;
    return `${Math.floor(seconds / 86_400)} days ago`;
  }

  /** A row's sentence, in the three pieces a time has to be an element to sit between. */
  interface QueueLine {
    lead: string;
    when?: { relative: string; exact: string; iso: string };
    tail?: string;
  }

  /**
   * What the row says about itself: what state it is in, why, and what happens next.
   *
   * One relative time per row, and the exact stamp rides that time's own tooltip - a
   * queue read at a glance is read in "in about four minutes", and a queue reasoned
   * about is read in a timestamp. Both, in two places, is what makes a row unreadable.
   *
   * A wait is said as a wait rather than as the state the service files it under: the
   * reason is what a reader can act on, and "Blocked" is a word about the queue.
   */
  function queueLine(item: QueueItem): QueueLine {
    const detail = item.summary ?? words(item.kind);
    const next = { relative: countdown(item.eligible_at), ...instant(item.eligible_at, item) };
    switch (item.state) {
      case 'awaiting_approval':
        return { lead: `${detail} · waiting for somebody to decide` };
      case 'running':
        return {
          lead:
            item.progress_total > 0
              ? `Running · ${item.progress_current} of ${item.progress_total} changes written`
              : 'Running',
        };
      case 'blocked':
        return { lead: `${item.blocked_reason ?? 'Waiting on something else'} · runs`, when: next };
      case 'retrying':
        return {
          lead: `${item.blocked_reason ?? `Attempt ${item.attempt} did not finish`} · tries again`,
          when: next,
          tail: ', on its own',
        };
      case 'succeeded':
      case 'failed':
      case 'cancelled':
      case 'superseded': {
        const finished = item.finished_at ?? item.updated_at;
        return {
          lead: `${detail} ·`,
          when: { relative: ago(finished), ...instant(finished, item) },
        };
      }
      default:
        return { lead: `${detail} · runs`, when: next };
    }
  }

  /** The exact instant a relative time is being relative about, said both ways. */
  function instant(value: string, item: QueueItem): { exact: string; iso: string } {
    return { exact: absolute(value, item.profile_timezone), iso: value };
  }

  /**
   * A standing, in words a reader owns - never the service's own state name.
   *
   * Nothing that the card's own heading already says: a row under "Needs a decision"
   * wearing a "Needs a decision" pill says it twice, and the states that are simply
   * what the group is called wear nothing at all.
   */
  function stateLabel(item: QueueItem): string | null {
    switch (item.state) {
      case 'running':
        return 'Running';
      case 'awaiting_approval':
      case 'blocked':
      case 'retrying':
      case 'scheduled':
      case 'ready':
        return null;
      default:
        return words(item.state);
    }
  }

  function stateTone(item: QueueItem): PillTone {
    if (item.state === 'failed') return 'danger';
    if (item.state === 'succeeded') return 'success';
    if (item.state === 'awaiting_approval' || item.state === 'running') return 'warning';
    return 'neutral';
  }

  function actionLabel(action: QueueActionType): string {
    if (action === 'next_window') return 'Next window';
    if (action === 'schedule_at') return 'Schedule exact time';
    if (action === 'set_priority') return 'Change priority';
    if (action === 'cancel') return 'Cancel work';
    return 'Run now';
  }

  function actionItems(item: QueueItem): ActionMenuItem[] {
    return (item.actions ?? [])
      .filter((action) => action !== 'run_now')
      .map(
        (action) =>
          ({
            id: action,
            label: actionLabel(action),
            description:
              action === 'next_window'
                ? 'Keep the assigned execution window'
                : action === 'schedule_at'
                  ? 'Choose the earliest acceptable time'
                  : action === 'set_priority'
                    ? 'Move this item to another priority band'
                    : 'Keep the item in audited history',
            icon:
              action === 'next_window'
                ? 'pending'
                : action === 'schedule_at'
                  ? 'history'
                  : action === 'set_priority'
                    ? 'sliders'
                    : 'trash',
            tone: action === 'cancel' ? 'danger' : 'default',
          }) satisfies ActionMenuItem,
      );
  }

  function selectMenuAction(item: QueueItem, action: string): void {
    if ((action as QueueMenuAction) === 'details') {
      onOpen(item);
      return;
    }
    onAction(item, action as QueueActionType);
  }
</script>

<!--
@component
The queue's work, in the three groups a reader asks about it in: what needs them, what
the service is doing on its own, and what it already did. A row is a sentence - what
this is, what state it is in, and what happens next - and it carries its act rather
than hiding it in a menu.

Rows animate, which is why they are `li`s with `animate:flip` and their own fades: work
arrives and retires while somebody is looking at it, and a row that appears without
moving reads as the page having been replaced.

Its clock is injected so a story or a test can hold time still. A countdown that reads
from the wall clock cannot be photographed.
-->

{#if grouped.length === 0}
  <div class="card">
    <div class="queue-empty">
      <strong>Nothing in this view</strong>
      <span>Queued work appears here as soon as the service accepts it.</span>
    </div>
  </div>
{:else}
  {#each grouped as group (group.id)}
    <div class="card queue-group">
      <div class="card-head">
        <h2 class="card-title">{groupTitle ?? group.title}</h2>
      </div>
      <ul class="object-list">
        {#each group.items as item (item.id)}
          <li animate:flip={rowMotion} in:fade={rowArriving} out:fade={rowLeaving}>
            <div class="object-row">
              <!-- The whole row opens the schedule, the progress and the audit
                   timeline; the acts stay pressable inside it. -->
              <button
                class="row-hit"
                type="button"
                aria-label="Open {item.title}"
                onclick={() => onOpen(item)}
              ></button>
              <span class="object-main">
                <span class="object-name-row">
                  <span class="object-name">{item.title}</span>
                  <span class="pill-swap">
                    {#key `${item.state}:${item.priority}:${item.revision}`}
                      <span class="pill-value" in:fade={valueMotion} out:fade={valueMotion}>
                        {#if stateLabel(item) !== null}
                          <Pill tone={stateTone(item)}>{stateLabel(item)}</Pill>
                        {/if}
                        {#if item.priority !== 'normal'}
                          <Pill tone={item.priority === 'urgent' ? 'danger' : 'neutral'}>
                            {words(item.priority)}
                          </Pill>
                        {/if}
                      </span>
                    {/key}
                  </span>
                </span>
                <span class="sum-swap">
                  {#key `${item.state}:${item.priority}:${item.revision}`}
                    {@const line = queueLine(item)}
                    <span class="object-sum" in:fade={valueMotion} out:fade={valueMotion}>
                      <!-- The separator rides the words, because markup whitespace
                           beside a block is trimmed and "runs" would take the time
                           straight onto its own last letter. -->
                      {line.when === undefined
                        ? line.lead
                        : `${line.lead} `}{#if line.when !== undefined}<time
                          datetime={line.when.iso}
                          title={line.when.exact}>{line.when.relative}</time
                        >{line.tail ?? ''}{/if}
                    </span>
                  {/key}
                </span>
              </span>
              <span class="object-side">
                {#if item.actions?.includes('run_now')}
                  <Button tone="signal" onclick={() => onAction(item, 'run_now')}>Run now</Button>
                {/if}
                {#if actionItems(item).length > 0}
                  <ActionMenu
                    label={`Actions for ${item.title}`}
                    items={actionItems(item)}
                    onSelect={(action) => selectMenuAction(item, action)}
                  />
                {:else}
                  <span class="row-chevron" aria-hidden="true">
                    <Icon name="chevron-right" size="xs" />
                  </span>
                {/if}
              </span>
            </div>
          </li>
        {/each}
      </ul>
    </div>
  {/each}
{/if}

<style>
  .queue-group + .queue-group {
    margin-block-start: var(--rhythm-card-gap);
  }

  .queue-empty {
    display: grid;
    gap: var(--space-1);
    padding: var(--space-6);
    text-align: center;
  }

  .queue-empty span {
    color: var(--text-muted);
  }

  .row-chevron {
    color: var(--text-muted);
    display: inline-grid;
    place-items: center;
  }

  /* Both readings share one cell, so the row keeps its height while they cross. */
  .pill-swap,
  .sum-swap {
    display: grid;
  }

  .pill-swap > .pill-value,
  .sum-swap > .object-sum {
    grid-area: 1 / 1;
  }

  .pill-value {
    align-items: center;
    display: flex;
    gap: var(--space-2);
  }

  /* A row with no standing to state must not pay the name row's gap for the swap it
     still carries. Both readings are present while they cross, so this asks whether
     EITHER of them holds a pill rather than whether the current one does. */
  .pill-swap:not(:has(:global(.pill))) {
    display: none;
  }
</style>

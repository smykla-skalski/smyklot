<script module lang="ts">
  import type { QueueItem } from '#lib/types.js';

  /** One card: a list, what it is called, and what it says about how much it holds. */
  export interface QueueCard {
    id: string;
    title: string;
    items: readonly QueueItem[];
    /** "Showing 1-N of TOTAL", for this card's own list and nobody else's. */
    count: string;
    more: boolean;
    busy: boolean;
    onMore: () => void;
  }
</script>

<script lang="ts">
  import { queueLine, words } from '#lib/queue-words.js';
  import type { QueueActionType } from '#lib/types.js';
  import { cubicOut } from 'svelte/easing';
  import { onMount } from 'svelte';
  import { flip } from 'svelte/animate';
  import { MediaQuery } from 'svelte/reactivity';
  import { fade } from 'svelte/transition';
  import ActionMenu, { type ActionMenuItem } from './ActionMenu.svelte';
  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import Chip, { type ChipTone } from './Chip.svelte';
  import Icon from './Icon.svelte';
  import Pill from './Pill.svelte';

  const {
    cards,
    workspace,
    clock = Date.now,
    reviewHref,
    onReview,
    onOpen,
    onAction,
  }: {
    cards: readonly QueueCard[];
    /**
     * Whose work a row is, where that is a question worth answering.
     *
     * The console reads every workspace at once, so its rows lead with the name; a
     * workspace's own queue passes nothing, because there the answer is the page.
     */
    workspace?: (item: QueueItem) => string | null;
    clock?: () => number;
    /**
     * Where a decision is actually made, for the rows that are waiting on one.
     *
     * A row that needs somebody carries the way to answer it rather than making them
     * find the page it lives on. Absent where there is no such page - the console
     * manages somebody else's sync and has no plan address of its own - and the row
     * falls back to opening its own detail.
     */
    reviewHref?: (item: QueueItem) => string | null;
    onReview?: (item: QueueItem, event: MouseEvent) => void;
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
   * What the work is about, in the words a person names it by.
   *
   * The name says what is being done and this says to what: a repository, and the
   * pull request inside it where there is one, led by the workspace where the page is
   * reading more than one. The owner is dropped from the repository either way - a
   * workspace is one owner - so the pill reads as a reference rather than as a path.
   */
  function subject(item: QueueItem): string | null {
    const where = workspace?.(item) ?? null;
    const repository = item.repository_name?.split('/').at(-1);
    if (repository === undefined || repository === '') return where;
    const pull = item.kind === 'pending_ci' ? item.details?.pull_request : undefined;
    const said = pull === undefined ? repository : `${repository} #${pull}`;

    return where === null ? said : `${where} · ${said}`;
  }

  /**
   * A standing, in words a reader owns - never the service's own state name.
   *
   * At the row's end rather than beside its name, and only where the row has no act
   * of its own: a row a reader can do something to says so with the act, and one they
   * cannot says how it ended. Nothing the card's own heading already says.
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

  /** What the standing MEANS, which is what a chip's tone names. */
  function stateTone(item: QueueItem): ChipTone {
    if (item.state === 'failed') return 'stop';
    if (item.state === 'succeeded') return 'clear';
    if (item.state === 'running' || item.state === 'ready') return 'signal';
    if (item.state === 'blocked' || item.state === 'retrying') return 'warning';
    if (item.state === 'awaiting_approval') return 'accent';

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
                ? "Keep the job's hours"
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

  /** Where this row's decision is made, or nothing where the reader cannot make it. */
  function review(item: QueueItem): string | null {
    if (item.state !== 'awaiting_approval') return null;

    return reviewHref?.(item) ?? null;
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

<div class="card-stack">
  {#if cards.length === 0}
    <Card>
      <div class="state-panel">
        <span
          ><strong>Nothing in this view.</strong> Queued work appears here as soon as the service accepts
          it</span
        >
      </div>
    </Card>
  {:else}
    {#each cards as card (card.id)}
      <Card class="queue-group">
        <div class="card-head">
          <h2 class="card-title">{card.title}</h2>
        </div>
        <ul class="object-list">
          {#each card.items as item (item.id)}
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
                    {#if subject(item) !== null}
                      <Pill>{subject(item)}</Pill>
                    {/if}
                    <span class="pill-swap">
                      {#key `${item.priority}:${item.revision}`}
                        <span class="pill-value" in:fade={valueMotion} out:fade={valueMotion}>
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
                      {@const line = queueLine(item, clock())}
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
                  <!-- ONE FILLED ACT ON THE PAGE, and it is the decision. Everything
                     else a row offers is a bordered control: a queue where every row
                     shouts has nothing left to say when one of them needs somebody. -->
                  <!-- `signal` is the filled one here: the mock's `.btn-brand` is a solid
                     fill of the action colour, and this sheet's `.btn-brand` is the
                     console's 12% tint of it. The look is what has to match. -->
                  {#if review(item) !== null}
                    <Button
                      tone="signal"
                      href={review(item) ?? ''}
                      onclick={(event) => onReview?.(item, event)}
                    >
                      Review and apply
                    </Button>
                  {/if}
                  {#if item.actions?.includes('run_now')}
                    <Button onclick={() => onAction(item, 'run_now')}>Run now</Button>
                  {/if}
                  {#if actionItems(item).length > 0}
                    <ActionMenu
                      label={`Actions for ${item.title}`}
                      items={actionItems(item)}
                      onSelect={(action) => selectMenuAction(item, action)}
                    />
                  {:else if stateLabel(item) !== null}
                    <!-- A row with nothing to act on says how it stands instead, at the
                       end where the act would have been. -->
                    <span class="state-swap">
                      {#key `${item.state}:${item.revision}`}
                        <span class="state-value" in:fade={valueMotion} out:fade={valueMotion}>
                          <Chip tone={stateTone(item)} dot={item.state === 'running'}>
                            {stateLabel(item)}
                          </Chip>
                        </span>
                      {/key}
                    </span>
                  {:else if review(item) === null && !(item.actions?.includes('run_now') ?? false)}
                    <!-- The way in, for a row that offers nothing else. A chevron beside
                       an act is a second way to say the row opens, and the act is the
                       one a reader came for. -->
                    <span class="row-chevron" aria-hidden="true">
                      <Icon name="chevron-right" size="xs" />
                    </span>
                  {/if}
                </span>
              </div>
            </li>
          {/each}
        </ul>
        <!-- ONLY WHERE THE CARD IS HOLDING SOMETHING BACK. A count under a list a reader
           can see the whole of answers a question nobody asked, and the rows above it
           already say how many there are. -->
        {#if card.more}
          <div class="list-foot">
            <span>{card.count}</span>
            <Button tone="quiet" disabled={card.busy} onclick={card.onMore}>Show more</Button>
          </div>
        {/if}
      </Card>
    {/each}
  {/if}
</div>

<style>
  .row-chevron {
    color: var(--text-muted);
    display: inline-grid;
    place-items: center;
  }

  /* Both readings share one cell, so the row keeps its height while they cross.
     One column that may not exceed the slot: an auto track is as wide as its widest
     line whatever the box around it says, so a long pull request title in the
     sentence drew straight over the acts at the end of the row. */
  .pill-swap,
  .state-swap,
  .sum-swap {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
  }

  .pill-swap > .pill-value,
  .state-swap > .state-value,
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

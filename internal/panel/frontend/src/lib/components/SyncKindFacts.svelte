<script module lang="ts">
  import type { SyncKind } from '../types';

  /**
   * What one kind is called in a sentence about it. The tree says "Repository
   * options" where the wire says `settings`, and a consequence line is prose -
   * it has to use the reader's word, not the column name.
   */
  const KIND_WORDS: Record<SyncKind, { one: string; changes: string }> = {
    labels: { one: 'label', changes: 'label changes' },
    settings: { one: 'repository option', changes: 'repository option changes' },
    rulesets: { one: 'ruleset', changes: 'ruleset changes' },
    files: { one: 'shared file', changes: 'shared file changes' },
  };

  /** The word beside a kind's switch: what it is doing, not what it would do. */
  export function syncSwitchWord(enabled: boolean): string {
    return enabled ? 'Syncing' : 'Paused';
  }

  /** What pressing the switch would do, for the reader who cannot see the track. */
  export function syncSwitchLabel(kind: SyncKind, enabled: boolean): string {
    return `${enabled ? 'Pause' : 'Resume'} ${KIND_WORDS[kind].one} syncing`;
  }
</script>

<!--
@component
Reach, delivery method and last saved change beside a sync switch. Saving a pause
stops future reconciliation; work already sent to GitHub remains there.
-->

<script lang="ts">
  import type { SyncStatus } from '../types';

  import RelativeTime from './RelativeTime.svelte';

  const {
    kind,
    enabled,
    status,
    updatedBy,
    updatedAt,
    nowMs,
  }: {
    kind: SyncKind;
    enabled: boolean;
    /** The fleet, for how many repositories this kind actually reaches. */
    status: SyncStatus | null;
    /** Who last changed this kind's configuration, or '' when nobody has. */
    updatedBy: string;
    updatedAt: string;
    nowMs: number;
  } = $props();

  /* A repository that switched this kind off in its own settings is not one
     the kind reaches, whatever the workspace's switch says. */
  const reach = $derived(
    status === null
      ? null
      : status.repositories.filter((row) => row.cells[kind].state !== 'off').length,
  );

  const said = $derived(
    enabled
      ? reach === null
        ? 'On'
        : `On for ${reach} ${reach === 1 ? 'repository' : 'repositories'}`
      : `Paused · existing repository changes are kept`,
  );

  const why = $derived(
    enabled
      ? kind === 'files'
        ? 'Pull requests open automatically'
        : 'Changes apply automatically'
      : 'Resume syncing to apply future changes',
  );

  const changed = $derived(updatedBy === '' || updatedAt === '' ? null : updatedAt);
</script>

<p class="switch-facts">
  <strong class="sf-status">{said}</strong><span class="sf-why">{why}</span
  >{#if changed !== null}<span class="sf-hist"
      >Changed by {updatedBy}
      <RelativeTime value={changed} {nowMs} /></span
    >{/if}
</p>

<style>
  /* Three roles on one line where there is room, joined by the separator's own
     ink rather than by the ink of whatever it separates. */
  .switch-facts {
    align-items: center;
    color: var(--text-secondary);
    display: flex;
    flex-wrap: wrap;
    font-size: var(--font-size-meta);
    gap: var(--space-1) var(--space-2);
    line-height: var(--leading-meta);
    margin: 0;
    text-wrap: pretty;
  }

  .sf-status {
    color: var(--text-primary);
    font-weight: 600;
  }

  .sf-hist {
    color: var(--text-muted);
  }

  /* THE SEPARATOR TRAILS WHAT IT FOLLOWS, never leads what it joins. Three
     roles need 771px and the band is usually narrower than that, so the line
     wraps on an ordinary desktop rather than only on a phone - and a leading
     `·` pushed onto the second line reads as a bullet, not as a join. Trailing,
     it ends a line the way a newspaper ends one, and the role that wrapped
     starts clean. */
  .switch-facts :is(.sf-status, .sf-why)::after {
    color: var(--text-muted);
    content: '·';
    font-weight: 400;
    margin-inline-start: var(--space-2);
  }

  /* A phone has no room for one line of three, so each role takes its own -
     and a separator ending a stacked line joins nothing. */
  @media (max-width: 47.9375rem) {
    .switch-facts {
      align-items: start;
      display: grid;
      gap: 2px;
    }

    .switch-facts :is(.sf-status, .sf-why)::after {
      content: none;
    }
  }
</style>

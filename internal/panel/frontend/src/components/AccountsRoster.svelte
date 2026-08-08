<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity';

  import { formatRelative, formatTimestamp } from '../lib/format';
  import { handleLabel, readHandle } from '../lib/identity';
  import type { PanelAccount, PanelPairing } from '../lib/types';
  import Avatar from './Avatar.svelte';
  import Chip from './Chip.svelte';
  import PairingList from './PairingList.svelte';
  import Plate from './Plate.svelte';

  const {
    accounts,
    pairings,
    viewerAccountId,
    onSetCanPair,
    onUnpair,
  }: {
    accounts: PanelAccount[];
    /**
     * Everyone else's pairings, folded into the account each belongs to rather
     * than listed apart, so somebody else's laptop is a fact about the person
     * who paired it. The viewer's own are deliberately absent: they have their
     * own card above, and a row that repeated them would be the same devices
     * under two headings.
     */
    pairings: PanelPairing[];
    /** So the owner can tell their own row from everyone else's. */
    viewerAccountId: string;
    onSetCanPair: (accountId: string, granted: boolean) => Promise<void>;
    onUnpair: (pairingId: string) => Promise<void>;
  } = $props();

  let working = $state<string | null>(null);
  /**
   * Accounts whose devices have been rolled down. Collapsed is the default.
   *
   * `SvelteSet` rather than a plain one so a membership change is something the
   * markup can see; a `Set` mutated in place is the same object, and nothing
   * downstream would re-render.
   */
  const opened = new SvelteSet<string>();

  const approved = $derived(accounts.filter((account) => account.can_pair).length);
  // A scan per row rather than one grouped pass. Both are over the pairings one
  // panel holds, which is a list a person reads, so the index would cost more to
  // explain than it saves.
  function devicesFor(accountId: string): PanelPairing[] {
    return pairings.filter((pairing) => pairing.account_id === accountId);
  }

  /**
   * Pairings the panel has no record of, so there is no account to fold them
   * into. Minted on the host, or left over from a database the panel has since
   * forgotten. Shown as their own group rather than dropped: an administrator
   * who cannot see a live credential cannot withdraw it.
   */
  const unattributed = $derived(pairings.filter((pairing) => pairing.account_id === undefined));
  const UNATTRIBUTED = 'unattributed';

  /**
   * Coarse enough for buckets that count whole minutes, often enough that a row
   * reading "seen just now" stops saying it. Captured once, the clock would
   * never move again for as long as the page stayed open.
   */
  const AGE_TICK_MS = 30_000;

  let now = $state(Date.now());

  $effect(() => {
    const tick = setInterval(() => {
      now = Date.now();
    }, AGE_TICK_MS);
    return () => {
      clearInterval(tick);
    };
  });

  function toggle(accountId: string): void {
    if (!opened.delete(accountId)) {
      opened.add(accountId);
    }
  }

  function devicesLabel(count: number): string {
    return count === 1 ? '1 device' : `${count} devices`;
  }

  async function decide(account: PanelAccount): Promise<void> {
    working = account.id;
    try {
      await onSetCanPair(account.id, !account.can_pair);
    } finally {
      working = null;
    }
  }
</script>

<Plate label="Accounts">
  {#snippet status()}
    <span class="tally mono">{approved} of {accounts.length} approved</span>
  {/snippet}

  {#if accounts.length === 0}
    <p class="dim">Nobody has signed in yet</p>
  {:else}
    <ul class="roster">
      {#each accounts as account (account.id)}
        {@const held = devicesFor(account.id)}
        {@const isOpen = opened.has(account.id)}
        <li class="row">
          <div class="head">
            <Avatar {account} size={34} />
            <div class="who">
              <p class="name">
                {account.display_name}
                {#if account.id === viewerAccountId}
                  <Chip small>You</Chip>
                {/if}
              </p>
              <!-- The state reads as the last fact about the account rather than
                   as a badge of its own. The relative time goes last because its
                   width changes as it ages, and anything after it would shift
                   sideways on the tick. -->
              <p class="meta mono">
                {handleLabel(readHandle(account.provider, account.login))}
                ·
                <span class:approved={account.can_pair}>
                  {account.can_pair ? 'approved' : 'awaiting approval'}
                </span>
                ·
                <span title={formatTimestamp(account.last_seen_at)}>
                  seen {formatRelative(account.last_seen_at, now)}
                </span>
              </p>
            </div>

            {#if held.length > 0}
              <button
                class="btn btn-quiet roll"
                aria-expanded={isOpen}
                aria-controls="devices-{account.id}"
                onclick={() => toggle(account.id)}
              >
                <span class="caret" class:caret-open={isOpen} aria-hidden="true"></span>
                {devicesLabel(held.length)}
              </button>
            {/if}

            <button
              class="btn decide"
              class:btn-stop={account.can_pair}
              disabled={working === account.id}
              onclick={() => decide(account)}
            >
              {account.can_pair ? 'Revoke' : 'Approve'}
            </button>
          </div>

          {#if isOpen}
            <div class="devices" id="devices-{account.id}">
              <PairingList pairings={held} {onUnpair} />
            </div>
          {/if}
        </li>
      {/each}

      {#if unattributed.length > 0}
        {@const isOpen = opened.has(UNATTRIBUTED)}
        <li class="row">
          <div class="head">
            <div class="who">
              <p class="name">Not from this panel</p>
              <p class="meta mono">minted on the host, or by a panel database since replaced</p>
            </div>
            <button
              class="btn btn-quiet roll"
              aria-expanded={isOpen}
              aria-controls="devices-{UNATTRIBUTED}"
              onclick={() => toggle(UNATTRIBUTED)}
            >
              <span class="caret" class:caret-open={isOpen} aria-hidden="true"></span>
              {devicesLabel(unattributed.length)}
            </button>
          </div>

          {#if isOpen}
            <div class="devices" id="devices-{UNATTRIBUTED}">
              <PairingList pairings={unattributed} {onUnpair} />
            </div>
          {/if}
        </li>
      {/if}
    </ul>
    <p class="footnote dim">
      Revoking stops new links. One already generated stays claimable until it expires, and a device
      already paired keeps working. Cut those off one at a time under the account that paired them
    </p>
  {/if}
</Plate>

<style>
  .tally {
    color: var(--dim);
    font-size: 0.6875rem;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  .roster {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .row {
    border-top: 1px solid var(--rule);
    padding: 0.75rem 0;
  }

  .row:first-child {
    border-top: 0;
    padding-top: 0;
  }

  .head {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem 0.75rem;
  }

  /* Takes the slack so every control lines up on the right. */
  .who {
    flex: 1 1 10rem;
    min-width: 0;
  }

  .name {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    font-weight: 600;
    gap: 0.4rem;
    margin: 0;
  }

  .meta {
    color: var(--dim);
    font-size: 0.6875rem;
    margin: 0.15rem 0 0;
    overflow-wrap: anywhere;
  }

  .approved {
    color: var(--clear);
  }

  .roll {
    flex: none;
    font-size: 0.8125rem;
    gap: 0.4rem;
  }

  /* Drawn rather than a glyph, so it turns rather than swapping character. */
  .caret {
    border-left: 4px solid currentcolor;
    border-top: 3.5px solid transparent;
    border-bottom: 3.5px solid transparent;
    height: 0;
    transition: transform 120ms ease-out;
    width: 0;
  }

  .caret-open {
    transform: rotate(90deg);
  }

  /* Inset and walled off, so the devices read as belonging to the row above
     rather than as the next entry in the roster. */
  .devices {
    border-left: 2px solid var(--rule);
    margin: 0.5rem 0 0 1.0625rem;
    padding-left: 1rem;
  }

  .decide {
    flex: none;
    width: 5.75rem;
  }

  .footnote {
    border-top: 1px solid var(--rule);
    font-size: 0.8125rem;
    margin: 0;
    padding-top: 0.875rem;
  }
</style>

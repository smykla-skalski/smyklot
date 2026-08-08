<script lang="ts">
  import type { PanelPairing } from '../lib/types';
  import PairingRow from './PairingRow.svelte';

  const {
    pairings,
    onUnpair,
  }: {
    pairings: PanelPairing[];
    onUnpair: (pairingId: string) => Promise<void>;
  } = $props();

  /**
   * Same cadence as the roster. Captured once, the clock would never move again
   * for as long as the page stayed open, so a row reading "claimed just now"
   * would go on saying it.
   */
  const AGE_TICK_MS = 30_000;

  let now = $state(Date.now());
  /** The row whose unpair has been asked for but not yet confirmed. */
  let confirming = $state<string | null>(null);
  let working = $state<string | null>(null);

  $effect(() => {
    const tick = setInterval(() => {
      now = Date.now();
    }, AGE_TICK_MS);
    return () => {
      clearInterval(tick);
    };
  });

  // A row that leaves the list takes its half-finished confirmation with it,
  // which otherwise reappears on whichever row later reuses the id.
  $effect(() => {
    if (confirming !== null && !pairings.some((pairing) => pairing.pairing_id === confirming)) {
      confirming = null;
    }
  });

  async function unpair(pairingId: string): Promise<void> {
    working = pairingId;
    try {
      await onUnpair(pairingId);
    } finally {
      working = null;
    }
    // Only this row's own confirmation, never whichever one happens to be open.
    if (confirming === pairingId) {
      confirming = null;
    }
  }
</script>

<ul class="rows">
  {#each pairings as pairing (pairing.pairing_id)}
    <PairingRow
      {pairing}
      {now}
      confirming={confirming === pairing.pairing_id}
      working={working === pairing.pairing_id}
      busy={working !== null}
      onConfirm={() => (confirming = pairing.pairing_id)}
      onCancel={() => (confirming = null)}
      onUnpair={() => unpair(pairing.pairing_id)}
    />

    {#if confirming === pairing.pairing_id}
      <!-- Beneath the row rather than in a dialog, so what is about to be cut
           off stays on screen next to the control that does it. -->
      <li class="warn" role="status">
        {pairing.device === undefined
          ? 'This link can no longer be claimed. It cannot be undone, and pairing that device means generating another link'
          : 'This device loses its access immediately. It cannot be undone, and pairing it again means a new link'}
      </li>
    {/if}
  {/each}
</ul>

<style>
  .rows {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  /* Tinted rather than plain text: it sits between two rows and has to read as
     belonging to the one above it. */
  .warn {
    background: var(--stop-tint);
    border-radius: var(--r-well);
    color: var(--stop);
    font-size: 0.8125rem;
    margin: 0 0 0.25rem;
    padding: 0.5rem 0.75rem;
  }
</style>

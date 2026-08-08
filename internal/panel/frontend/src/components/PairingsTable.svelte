<script lang="ts">
  import { liveCount } from '../lib/pairing-state';
  import type { PanelPairing } from '../lib/types';
  import PairingList from './PairingList.svelte';
  import Plate from './Plate.svelte';

  const {
    pairings,
    failure,
    onUnpair,
  }: {
    /**
     * The viewer's own, and only theirs. An administrator reaches everybody
     * else's through the account each one belongs to, where the person is
     * already named and the devices are one fact about them rather than a list
     * of strangers' laptops.
     */
    pairings: PanelPairing[];
    failure: string | null;
    onUnpair: (pairingId: string) => Promise<void>;
  } = $props();

  const live = $derived(liveCount(pairings));
</script>

<Plate label="Your devices">
  {#snippet status()}
    <span class="tally mono">{live} of {pairings.length} live</span>
  {/snippet}

  {#if failure !== null}
    <p class="failure">{failure}</p>
  {/if}

  {#if pairings.length === 0}
    <p class="dim">You have not paired a device yet</p>
  {:else}
    <PairingList {pairings} {onUnpair} />
  {/if}
</Plate>

<style>
  .tally {
    color: var(--dim);
    font-size: 0.6875rem;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  .failure {
    color: var(--stop);
    margin: 0 0 0.875rem;
  }
</style>

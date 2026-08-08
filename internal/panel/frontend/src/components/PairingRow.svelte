<script lang="ts">
  import { formatRelative, formatTimestamp } from '../lib/format';
  import {
    pairingCanUnpair,
    pairingChange,
    pairingIsLive,
    pairingSubject,
    pairingTone,
  } from '../lib/pairing-state';
  import type { PanelPairing } from '../lib/types';
  import Chip from './Chip.svelte';

  const {
    pairing,
    now,
    confirming,
    working,
    busy,
    onConfirm,
    onCancel,
    onUnpair,
  }: {
    pairing: PanelPairing;
    now: number;
    /** Whether this row is the one waiting for its unpair to be confirmed. */
    confirming: boolean;
    /** Whether this row's own unpair is in flight. */
    working: boolean;
    /** Whether any row's unpair is in flight, which locks every control. */
    busy: boolean;
    onConfirm: () => void;
    onCancel: () => void;
    onUnpair: () => void;
  } = $props();

  const change = $derived(pairingChange(pairing));
</script>

<li class="row">
  <!-- Fixed width rather than shrink-to-fit, so REVOKED and ACTIVE leave the
       name starting at the same place down the whole list. -->
  <span class="badge">
    <Chip tone={pairingTone(pairing.state)} dot={pairingIsLive(pairing.state)}>
      {pairing.state}
    </Chip>
  </span>

  <!-- A device has a name somebody gave it; a link without one has only a
       sentence about why. Set apart because at the same weight the sentence
       reads as another device, and the list stops being a list of things. -->
  <p class="name" class:stands-in={pairing.device === undefined}>{pairingSubject(pairing)}</p>

  <!-- On the name's line rather than beneath it: one device is one row, and a
       second line per device turns a short list into a page of its own. -->
  <p class="meta mono">
    {#if pairing.device !== undefined}
      {pairing.device.platform}
      <span aria-hidden="true">·</span>
    {/if}
    <span title={formatTimestamp(change.at)}>
      {change.label}
      {formatRelative(change.at, now)}
    </span>
  </p>

  {#if confirming}
    <span class="decide">
      <button class="btn btn-row btn-stop" disabled={working} onclick={onUnpair}>
        {working ? 'Unpairing…' : 'Confirm'}
      </button>
      <button class="btn btn-row btn-quiet" disabled={working} onclick={onCancel}>Cancel</button>
    </span>
  {:else if pairingCanUnpair(pairing.state)}
    <!-- Disabled while any row is being unpaired, not just this one. Opening a
         second confirmation would close the running row's, and the request that
         finished first would hand every control back while the other was still
         in flight. -->
    <button class="btn btn-row unpair" disabled={busy} onclick={onConfirm}>Unpair</button>
  {:else}
    <!-- A finished pairing has no unpair worth offering, but taking the column
         away would let its own line run on past every other row's. -->
    <span class="unpair" aria-hidden="true"></span>
  {/if}
</li>

<style>
  .row {
    align-items: center;
    border-top: 1px solid var(--rule);
    display: flex;
    gap: 0.75rem;
    padding: 0.5rem 0;
  }

  .row:first-child {
    border-top: 0;
  }

  /* Wide enough for the longest state the daemon spells, and no wider: the gap
     it leaves in front of every shorter one is the price of the alignment.
     A flex container rather than a plain span, because an inline wrapper sits
     the chip on a text baseline and the leading above it pushes the chip about
     2px below where centring the wrapper says it is. */
  .badge {
    align-items: center;
    display: flex;
    flex: none;
    width: 5.75rem;
  }

  /* Trimmed to the cap band so the row centres what a reader sees rather than
     the line box, which carries descender space these lines never use. Without
     it the text sits about a pixel below the chip beside it. Browsers without
     `text-box` fall back to that same pixel, which is what this looked like
     before. */
  .name,
  .meta {
    line-height: 1;
    margin: 0;
    text-box: trim-both cap alphabetic;
  }

  .name {
    flex: 1 1 auto;
    font-weight: 600;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    /* The clip is what gives the ellipsis, and a box trimmed to the cap band
       would cut every descender off with it. The padding gives them room and
       the matching negative margin takes it back out of the margin box, so the
       cap band is still what the row centres on. */
    padding-block: 0.35em;
    margin-block: -0.35em;
  }

  .stands-in {
    color: var(--dim);
    font-weight: 400;
  }

  /* Never the one that gives way: it is already the shortest thing here, and a
     truncated timestamp says less than no timestamp at all. */
  .meta {
    color: var(--dim);
    flex: none;
    font-size: 0.6875rem;
    text-align: right;
  }

  .decide {
    display: flex;
    flex: none;
    gap: 0.25rem;
  }

  /* The height as well as the width, because the spacer that stands in for a
     finished pairing's missing control is what sets that row's height, and
     without it those rows sit shorter than the ones around them. */
  .unpair {
    flex: none;
    min-height: 1.75rem;
    width: 4.5rem;
  }

  /* Below this the single line stops being possible without truncating the
     device name to nothing, so the meta drops under it and the row wraps. */
  @media (max-width: 34rem) {
    .row {
      flex-wrap: wrap;
      gap: 0.35rem 0.5rem;
    }

    .badge {
      width: auto;
    }

    .name {
      flex: 1 1 100%;
      order: 2;
    }

    .meta {
      order: 3;
      text-align: left;
    }
  }
</style>

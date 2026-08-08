<script lang="ts">
  import { prefersReducedMotion } from 'svelte/motion';
  import { slide } from 'svelte/transition';

  import type { ClipboardWriter } from '../lib/clipboard';
  import { copyText } from '../lib/clipboard';
  import { remainingFraction, remainingMs } from '../lib/format';
  import { pairingHref } from '../lib/pairing';
  import type { PairLink, PanelPairing } from '../lib/types';
  import Chip from './Chip.svelte';
  import PairLinkGauge from './PairLinkGauge.svelte';
  import Plate from './Plate.svelte';

  const {
    canPair,
    pairings,
    onGenerate,
    onCancel,
    onLapse,
  }: {
    canPair: boolean;
    /**
     * The viewer's own pairings, which is how the card learns what became of the
     * link it is showing. It finds itself by id rather than being told, so the
     * page above does not have to hold a second copy of which link is on screen.
     */
    pairings: PanelPairing[];
    onGenerate: () => Promise<PairLink>;
    /**
     * Withdraw a link nobody has claimed. Dropping it from the page alone would
     * leave it claimable by whoever already has it for the rest of its life,
     * which is the opposite of what the control appears to do.
     */
    onCancel: (pairingId: string) => Promise<void>;
    /**
     * The shown link just ran out. Nothing announces an expiry — it is a
     * deadline passing rather than anything happening — so the page is told from
     * here, where the countdown that reached it lives.
     */
    onLapse: () => void;
  } = $props();

  const COPIED_SETTLE_MS = 2_500;
  const TICK_MS = 1_000;
  /**
   * How long to let the handoff prove itself before offering the manual route.
   * Long enough for the operating system to bring an installed app forward, short
   * enough that somebody stuck is not left watching a page that says nothing.
   */
  const HANDOFF_GRACE_MS = 1_600;
  /** How long the card takes to roll down. Zero for anyone who asked for still. */
  const SLIDE_MS = $derived(prefersReducedMotion.current ? 0 : 180);

  let link = $state<PairLink | null>(null);
  /**
   * When the link arrived. The reply carries a deadline but no start, and the
   * drain track has to measure the remainder against a whole lifetime.
   */
  let issuedMs = $state(0);
  let nowMs = $state(0);
  let working = $state(false);
  let cancelling = $state(false);
  let failure = $state<string | null>(null);
  let copyState = $state<'idle' | 'copied' | 'manual'>('idle');
  let handoff = $state<'idle' | 'waiting' | 'stalled'>('idle');
  let field = $state<HTMLInputElement | null>(null);
  let handoffControl = $state<HTMLAnchorElement | null>(null);

  /**
   * The device that claimed the link this card is showing, once the daemon has
   * said so.
   *
   * From the pairing list rather than from anything the page guessed: a link is
   * spent by a device the browser never hears from, and until the daemon reports
   * the claim the link on screen still works. Retiring it any earlier would take
   * a live credential off the one page that can ever show it.
   */
  const claimedBy = $derived.by(() => {
    const current = link;
    if (current === null) {
      return null;
    }
    const mine = pairings.find((pairing) => pairing.pairing_id === current.pairing_id);
    return mine?.device ?? null;
  });

  const leftMs = $derived(link === null ? null : remainingMs(link.expires_at, nowMs));
  const expired = $derived(leftMs === 0);
  const href = $derived(link === null ? null : pairingHref(link.pairing_url));
  const left = $derived.by(() => {
    if (link === null || leftMs === null) {
      return 0;
    }
    return remainingFraction(issuedMs, Date.parse(link.expires_at), nowMs);
  });

  const copyNote = $derived.by(() => {
    switch (copyState) {
      case 'copied':
        return 'Copied to the clipboard';
      case 'manual':
        return 'This browser would not let the page copy. The link is selected; press Cmd-C or Ctrl-C';
      case 'idle':
        return '';
    }
  });

  // Depends on the link alone. Reading the countdown here instead would tear the
  // interval down and rebuild it on every tick. What ends it early is decided
  // inside the tick below, for the same reason.
  $effect(() => {
    const current = link;
    if (current === null) {
      return;
    }
    const deadline = Date.parse(current.expires_at);
    nowMs = Date.now();
    // A deadline that cannot be read has nothing to count down to. Ticking anyway
    // would re-render every second behind a label that never changes.
    if (!Number.isFinite(deadline)) {
      return;
    }
    const tick = setInterval(() => {
      // A claimed link has nothing left to count down. Stopping from inside the
      // tick rather than by keying the effect on the claim keeps the interval
      // built once per link: the claim arrives on a whole new pairing list, so
      // as a dependency it would tear this down and rebuild it on every read.
      if (claimedBy !== null) {
        clearInterval(tick);
        return;
      }
      nowMs = Date.now();
      if (nowMs < deadline) {
        return;
      }
      clearInterval(tick);
      onLapse();
    }, TICK_MS);
    return () => {
      clearInterval(tick);
    };
  });

  $effect(() => {
    if (copyState !== 'copied') {
      return;
    }
    const settle = setTimeout(() => {
      copyState = 'idle';
    }, COPIED_SETTLE_MS);
    return () => {
      clearTimeout(settle);
    };
  });

  /**
   * Whether the handoff went anywhere cannot be asked directly, so this watches
   * for the page losing the foreground and treats staying put as a reason to
   * offer the manual route. A browser prompting for permission also stays put, so
   * the hint is written as a possibility rather than as a verdict.
   */
  $effect(() => {
    if (handoff !== 'waiting') {
      return;
    }
    const settle = setTimeout(() => {
      handoff = 'stalled';
    }, HANDOFF_GRACE_MS);
    const departed = (): void => {
      handoff = 'idle';
    };
    // `blur` is the one that fires on macOS and Windows, where another app coming
    // forward leaves the tab visible; `visibilitychange` covers a mobile browser
    // that backgrounds the whole page instead.
    window.addEventListener('blur', departed);
    document.addEventListener('visibilitychange', departed);
    return () => {
      clearTimeout(settle);
      window.removeEventListener('blur', departed);
      document.removeEventListener('visibilitychange', departed);
    };
  });

  async function generate(): Promise<void> {
    working = true;
    failure = null;
    copyState = 'idle';
    handoff = 'idle';
    try {
      const minted = await onGenerate();
      issuedMs = Date.now();
      nowMs = issuedMs;
      link = minted;
    } catch (error) {
      failure = error instanceof Error ? error.message : String(error);
    } finally {
      working = false;
    }
  }

  /**
   * Withdraw the link and put the card away.
   *
   * The card closes only once the daemon has agreed. Closing first would leave
   * whoever holds the link able to claim it, behind a page that says the pairing
   * was cancelled.
   */
  async function cancel(): Promise<void> {
    const current = link;
    if (current === null) {
      return;
    }
    cancelling = true;
    failure = null;
    try {
      await onCancel(current.pairing_id);
      // Only if this is still the link that was withdrawn. A mint that landed
      // while the revoke was in flight owns the card now, and clearing it would
      // drop a live link off a page that can never show it again.
      if (link === current) {
        link = null;
        copyState = 'idle';
        handoff = 'idle';
      }
    } catch (error) {
      failure = error instanceof Error ? error.message : String(error);
    } finally {
      cancelling = false;
    }
  }

  /**
   * Put a claimed card away.
   *
   * Nothing is withdrawn: the link has already been spent and the device it
   * became belongs in the list below, where cutting it off is a decision of its
   * own rather than the price of closing a card.
   */
  function dismiss(): void {
    link = null;
    copyState = 'idle';
    handoff = 'idle';
  }

  // The handoff is the next thing to do, and putting focus on it lets the whole
  // flow finish from the keyboard without hunting for the control. Keyed on the
  // link alone: reading the countdown here would pull focus back every second.
  $effect(() => {
    if (link !== null) {
      handoffControl?.focus();
    }
  });

  async function copy(): Promise<void> {
    if (link === null) {
      return;
    }
    const writer: ClipboardWriter | undefined = navigator.clipboard;
    if ((await copyText(link.pairing_url, writer)) === 'copied') {
      copyState = 'copied';
      return;
    }
    // Nothing else can reach the clipboard, so hand over the selection and let
    // the person finish it themselves.
    copyState = 'manual';
    field?.focus();
    field?.select();
  }
</script>

{#if !canPair}
  <Plate label="Pair a device" tone="lead">
    {#snippet status()}
      <Chip>Awaiting approval</Chip>
    {/snippet}
    <p class="dim">
      Ask an administrator to approve this account. Once they have, you can generate a link here
    </p>
  </Plate>
{:else if link === null}
  <!-- One control until there is something to show. The card it opens exists to
       hold a live credential, and standing it up empty put a paragraph and a
       button inside a frame that said nothing the button did not. -->
  <div class="start">
    <button class="btn btn-signal" onclick={generate} disabled={working}>
      {working ? 'Generating…' : 'Pair a device'}
    </button>
    {#if failure !== null}
      <p class="failure">{failure}</p>
    {/if}
  </div>
{:else}
  <div class="open" transition:slide={{ duration: SLIDE_MS }}>
    <Plate label="Pair a device" tone="lead">
      <!-- In the header rather than over the link: it is a caveat about the
           thing below, and as a full-width paragraph it read as the first
           instruction instead. -->
      {#snippet status()}
        {#if claimedBy !== null}
          <Chip tone="clear" dot>Paired</Chip>
        {:else if !expired}
          <span class="caveat">This link is shown once and cannot be shown again</span>
        {/if}
      {/snippet}

      {#if claimedBy !== null}
        <!-- The value, the copy control, and the countdown go together, because
             the link is spent and each of them on its own would offer something
             that can no longer do anything. What stands where they were is the
             answer the person was waiting for: which device took it. -->
        <p class="paired" role="status">
          <span class="device">{claimedBy.display_name}</span> is paired
        </p>
        <p class="aside dim">It is in your devices below, where you can cut it off at any time</p>
      {:else}
        <div class="ticket" class:ticket-spent={expired}>
          <!-- Selected on focus so it can be copied in one gesture even where the
             clipboard is refused, and readonly so an accidental edit cannot produce
             a link that looks right and is not. The value is a one-time code, so the
             browser is told to keep it out of form history and away from the spell
             checker, which in some browsers means a remote service. -->
          <div class="well">
            <input
              bind:this={field}
              class="value mono"
              type="text"
              readonly
              autocomplete="off"
              autocorrect="off"
              autocapitalize="off"
              spellcheck="false"
              value={link.pairing_url}
              aria-label="Pairing link"
              onfocus={(event) => event.currentTarget.select()}
            />
            <button class="btn copy" onclick={copy} disabled={expired}>
              {copyState === 'copied' ? 'Copied' : 'Copy'}
            </button>
          </div>

          <PairLinkGauge {leftMs} fractionLeft={left} expiresAt={link.expires_at} />
        </div>

        <p class="note" class:note-quiet={copyState !== 'manual'} role="status">{copyNote}</p>

        {#if expired}
          <p>This link lapsed before anything claimed it. Generate another to pair the device</p>
        {/if}
      {/if}

      <!-- Leftmost is the way out, rightmost is the way on. The handoff sat on
           the left before, where a control in the corner of a card that has just
           rolled down reads as the thing that rolls it back up. -->
      <div class="actions">
        {#if claimedBy === null}
          <button class="btn btn-quiet" onclick={cancel} disabled={working || cancelling}>
            {cancelling ? 'Cancelling…' : 'Cancel'}
          </button>
        {:else}
          <!-- Not a cancel. That control withdraws the pairing, and pressing it
               here would cut off the device that has just paired. -->
          <button class="btn btn-quiet" onclick={dismiss} disabled={working}>Done</button>
        {/if}
        <!-- Also while a cancel is in flight: minting into a card that is being
             withdrawn races the two, and the loser is a live link nobody sees. -->
        <button
          class="btn onward"
          class:btn-signal={expired}
          onclick={generate}
          disabled={working || cancelling}
        >
          {#if working}
            Generating…
          {:else if claimedBy !== null}
            Pair another device
          {:else}
            Generate another
          {/if}
        </button>
        {#if href !== null && !expired && claimedBy === null}
          <!-- `harness://` is registered by the Harness apps, so on the device
               being paired this finishes the job in one press. An anchor rather
               than a scripted navigation: it works from the keyboard, carries the
               browser's own copy-link menu, and is not mistaken for a popup. -->
          <a
            bind:this={handoffControl}
            class="btn btn-signal"
            {href}
            onclick={() => (handoff = 'waiting')}
          >
            Open in Harness Monitor
          </a>
        {/if}
      </div>

      <!-- Only when the handoff went nowhere. Saying the same thing up front made
           every reader answer a question they had not asked yet. -->
      {#if !expired && claimedBy === null && handoff === 'stalled'}
        <p class="aside dim" role="status">
          Harness Monitor did not come forward. If it is not installed on this device, copy the link
          and open it on the one you want to pair
        </p>
      {/if}

      {#if failure !== null}
        <p class="failure">{failure}</p>
      {/if}
    </Plate>
  </div>
{/if}

<style>
  /* Carries the plate's own bottom margin, which the card inside it would
     otherwise be the only thing on the page not to have. */
  .start {
    margin-bottom: 1rem;
  }

  .caveat {
    color: var(--dim);
    font-size: 0.8125rem;
    text-align: right;
  }

  .ticket {
    margin-bottom: 1rem;
  }

  /* Where the link strip was, at the weight the strip had: this is the one line
     the person opened the card to read. */
  .paired {
    font-size: 1rem;
    margin: 0 0 0.25rem;
  }

  .device {
    font-weight: 600;
  }

  /* A solid tab rather than an outline: this is the one thing on the page holding
     a live credential, and it should read as a physical strip. */
  .well {
    align-items: center;
    background: var(--well);
    border-left: 3px solid var(--signal);
    border-radius: var(--r-well);
    display: flex;
    gap: 0.5rem;
    padding: 0.375rem 0.375rem 0.375rem 0.75rem;
  }

  .value {
    background: transparent;
    border: 0;
    color: var(--text);
    flex: 1;
    font-size: 0.8125rem;
    min-width: 0;
    padding: 0.25rem 0;
  }

  .value:focus {
    outline: none;
  }

  .copy {
    background: var(--strip);
    flex: none;
    min-height: 1.875rem;
    padding: 0 0.75rem;
  }

  .ticket-spent .well {
    border-left-color: var(--rule);
  }

  .ticket-spent .value {
    color: var(--dim);
  }

  .actions {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
  }

  /* Everything from here rightward is the way on, pushed away from the way out. */
  .onward {
    margin-left: auto;
  }

  /* Kept in the accessibility tree for the confirmation the button already shows
     visually, so it is announced once without repeating what is on screen. */
  .note {
    font-size: 0.8125rem;
    margin: 0 0 0.875rem;
  }

  .note-quiet {
    clip-path: inset(50%);
    height: 1px;
    overflow: hidden;
    position: absolute;
    white-space: nowrap;
    width: 1px;
  }

  .aside {
    font-size: 0.8125rem;
    margin: 0.75rem 0 0;
  }

  .failure {
    color: var(--stop);
    margin: 0.875rem 0 0;
  }
</style>

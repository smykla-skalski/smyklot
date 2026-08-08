<script lang="ts">
  import AccountsRoster from './components/AccountsRoster.svelte';
  import IdentityBar from './components/IdentityBar.svelte';
  import PageFooter from './components/PageFooter.svelte';
  import PairingsTable from './components/PairingsTable.svelte';
  import PairLinkPanel from './components/PairLinkPanel.svelte';
  import Plate from './components/Plate.svelte';
  import SignedOut from './components/SignedOut.svelte';
  import type { PanelApi } from './lib/api';
  import type { PanelBuild } from './lib/base';
  import type { PairLink, PanelAccount, PanelPairing, PanelViewer } from './lib/types';

  const { api, iconUrl, build }: { api: PanelApi; iconUrl: string; build: PanelBuild } = $props();

  let loading = $state(true);
  let viewer = $state<PanelViewer | null>(null);
  let accounts = $state<PanelAccount[]>([]);
  let pairings = $state<PanelPairing[]>([]);
  /**
   * Kept from the last successful read rather than cleared alongside the
   * pairings: a daemon that stops answering has not changed version, and
   * blanking the footer mark would read as one.
   */
  let daemonVersion = $state<string | null>(null);
  let failure = $state<string | null>(null);
  /**
   * Kept apart from `failure`, because this one comes from the daemon rather
   * than from the panel. A daemon that cannot be reached should not make the
   * identity bar and the roster disappear behind a page-wide problem.
   */
  let pairingsFailure = $state<string | null>(null);

  async function load(): Promise<void> {
    // Only the first load blanks the page. A later refresh keeps what is on
    // screen, because tearing the tree down would destroy the shown-once
    // pairing link that nothing else holds a copy of.
    loading = viewer === null;
    failure = null;
    try {
      viewer = await api.fetchViewer();
      // Only the owner may list accounts, so asking as anyone else would turn
      // an ordinary page load into a 403 the person cannot act on.
      accounts = viewer?.is_owner === true ? await api.fetchAccounts() : [];
    } catch (error) {
      failure = error instanceof Error ? error.message : String(error);
    } finally {
      loading = false;
    }
    if (viewer !== null) {
      await loadPairings();
    }
  }

  /**
   * Counts reads of the pairings so only the newest may apply what it found.
   *
   * These overlap: a mint refreshes without waiting, an unpair refreshes after
   * it, and both can be in flight while the page reloads. Two reads answering
   * out of order would otherwise leave the older one's snapshot on screen,
   * showing a device as still paired seconds after it was cut off. Signing out
   * bumps it too, so a read that started under one session cannot land under
   * the next.
   */
  let pairingsRead = 0;

  async function loadPairings(): Promise<void> {
    const read = ++pairingsRead;
    pairingsFailure = null;
    try {
      const listed = await api.fetchPairings();
      if (read === pairingsRead) {
        pairings = listed.pairings;
        daemonVersion = listed.daemon_version ?? null;
      }
    } catch (error) {
      if (read === pairingsRead) {
        pairingsFailure = error instanceof Error ? error.message : String(error);
      }
    }
  }

  /**
   * Re-read rather than patch the row in place: the daemon is the authority on
   * what a pairing became, and one revoked elsewhere in the meantime should
   * settle at what it really is rather than at what this call did.
   */
  async function unpair(pairingId: string): Promise<void> {
    try {
      await api.revokePairing(pairingId);
    } catch (error) {
      pairingsFailure = error instanceof Error ? error.message : String(error);
      return;
    }
    await loadPairings();
  }

  /** Mint, then show the new link in the table without waiting for a reload. */
  async function generate(): Promise<PairLink> {
    const link = await api.createPairLink();
    void loadPairings();
    return link;
  }

  /**
   * Re-read on anything the panel pushes, rather than patching the row the event
   * carries.
   *
   * The daemon is the authority on what a pairing became, and a read settles the
   * whole page at one instant: the event that arrives is a fact about one row,
   * but a claim also spends a link, and a page that patched only the row named
   * in the event would leave the rest of what changed with it untouched. It also
   * keeps one path into `pairings`, so the freshness guard above covers the
   * pushed case for free.
   */
  $effect(() => {
    if (viewer === null) {
      return;
    }
    return api.openStream({
      onResync: () => void loadPairings(),
      onPairing: () => void loadPairings(),
    });
  });

  /**
   * Withdraw a link the card is still showing. The same route an unpair uses,
   * because a link nobody claimed and a device nobody wants are the same thing
   * to the daemon. The failure is deliberately rethrown: the card is where the
   * control was pressed and where the reason belongs.
   */
  async function cancelLink(pairingId: string): Promise<void> {
    await api.revokePairing(pairingId);
    void loadPairings();
  }

  async function setCanPair(accountId: string, granted: boolean): Promise<void> {
    try {
      await api.setCanPair(accountId, granted);
      // Re-read rather than patching in place: the decision may have changed
      // the viewer's own row, and the server is the authority on both. `load`
      // leaves the page standing, so a link already on screen survives.
      await load();
    } catch (error) {
      failure = error instanceof Error ? error.message : String(error);
    }
  }

  async function signOut(): Promise<void> {
    loading = true;
    failure = null;
    pairingsFailure = null;
    viewer = null;
    accounts = [];
    pairings = [];
    daemonVersion = null;
    // Discards any read already in flight, which would otherwise resolve during
    // the sign-out below and put this session's devices back on the page.
    pairingsRead += 1;
    try {
      await api.signOut();
    } catch (error) {
      const signOutFailure = error instanceof Error ? error.message : String(error);
      await load();
      failure = signOutFailure;
      return;
    }
    await load();
  }

  /**
   * The viewer's own devices. An administrator receives everyone's, but their
   * own card should hold theirs alone; the rest belong to the account rows,
   * where each one is a fact about the person who paired it.
   */
  const mine = $derived.by(() => {
    const signedIn = viewer;
    if (signedIn === null) {
      return [];
    }
    return pairings.filter((pairing) => pairing.account_id === signedIn.account.id);
  });

  /** Everyone else's, which the roster folds into the account each belongs to. */
  const theirs = $derived.by(() => {
    const signedIn = viewer;
    if (signedIn === null) {
      return [];
    }
    return pairings.filter((pairing) => pairing.account_id !== signedIn.account.id);
  });

  void load();
</script>

<main class="shell">
  <IdentityBar {viewer} {iconUrl} onSignOut={signOut} />

  {#if failure !== null}
    <Plate label="Problem" tone="alarm">
      <p>{failure}</p>
      <button class="btn" onclick={load}>Try again</button>
    </Plate>
  {/if}

  {#if loading}
    <Plate label="Panel">
      <p class="dim">Reading the panel…</p>
    </Plate>
  {:else if viewer !== null}
    <!-- The viewer's own pairings, so the card can find the link it is showing
         and retire it when the daemon reports the claim. -->
    <PairLinkPanel
      canPair={viewer.account.can_pair}
      pairings={mine}
      onGenerate={generate}
      onCancel={cancelLink}
      onLapse={loadPairings}
    />
    <!-- Skipped only for someone who has nothing paired and cannot pair
         anything: for them the control above already says what to do, and an
         empty table underneath repeats it. A failure still shows the plate,
         because the alternative is swallowing it where nobody can see it. -->
    {#if mine.length > 0 || viewer.account.can_pair || pairingsFailure !== null}
      <PairingsTable pairings={mine} failure={pairingsFailure} onUnpair={unpair} />
    {/if}
    {#if viewer.is_owner}
      <AccountsRoster
        {accounts}
        pairings={theirs}
        viewerAccountId={viewer.account.id}
        onSetCanPair={setCanPair}
        onUnpair={unpair}
      />
    {/if}
    <!-- A failed load proves nothing about whether anyone is signed in, so the
       gate stays away: offering sign-in as the way out of a daemon outage sends
       someone to repeat what they have already done. -->
  {:else if failure === null}
    <SignedOut href={api.signInUrl()} />
  {/if}

  <PageFooter {build} {daemonVersion} />
</main>

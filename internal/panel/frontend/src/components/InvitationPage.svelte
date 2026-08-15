<script lang="ts">
  import type { PanelApi } from '../lib/api';
  import type { PanelBuild } from '../lib/base';
  import { formatDateTime } from '../lib/format';
  import type { PanelInvitation } from '../lib/types';
  import Avatar from './Avatar.svelte';
  import BrandMark from './BrandMark.svelte';
  import Chip, { type ChipTone } from './Chip.svelte';
  import NightSky from './NightSky.svelte';
  import PageFooter from './PageFooter.svelte';

  const { api, token, build }: { api: PanelApi; token: string; build: PanelBuild } = $props();

  /* One source for the mark's size: the component needs the number, and the sky
     needs it in CSS to find the middle of the mark it opens out from. */
  const MARK_SIZE = 96;

  let invitation = $state<PanelInvitation | null>(null);
  let loading = $state(true);
  let failure = $state<string | null>(null);

  /* The skeleton stands in for an answer the page does not have yet. Once it has
     one, a retry keeps it on screen and marks the card busy: swapping it back to
     a placeholder of a different height moves the whole centred stack, twice. */
  const nothingYet = $derived(invitation === null && failure === null);

  /* The card carries no header of its own, so its title stands above it and
     names whichever of the three states the card is showing. It follows what is
     displayed rather than the request, so a retry does not flicker it. */
  const title = $derived(
    invitation !== null
      ? 'GitHub access invitation'
      : failure !== null
        ? 'Invitation unavailable'
        : 'Invitation',
  );

  $effect(() => {
    void load(token);
  });

  async function load(requestedToken: string): Promise<void> {
    loading = true;
    try {
      invitation = await api.fetchInvitation(requestedToken);
      failure = null;
    } catch (error) {
      failure = error instanceof Error ? error.message : String(error);
      invitation = null;
    } finally {
      loading = false;
    }
  }

  function statusTone(status: PanelInvitation['status']): ChipTone {
    if (status === 'accepted') return 'clear';
    if (status === 'pending') return 'signal';
    if (status === 'expired') return 'warning';
    return 'stop';
  }

  function roleLabel(value: PanelInvitation): string {
    if (value.system_role === 'root') return 'Root';
    const role = value.role ?? 'viewer';
    return role.slice(0, 1).toUpperCase() + role.slice(1);
  }
</script>

<svelte:head>
  <title>Invitation | SMYKLOT</title>
</svelte:head>

<main class="shell invitation-shell">
  <div class="invitation-brand" style="--invitation-mark-size: {MARK_SIZE}px">
    <NightSky />
    <BrandMark part="PANEL" stacked size={MARK_SIZE} />
  </div>

  <div class="invitation-main">
    <h1 class="invitation-title" id="invitation-title">{title}</h1>

    <section
      class={['plate', 'invitation-card', loading && 'loading']}
      aria-labelledby="invitation-title"
      aria-busy={loading}
    >
      <div class="plate-body">
        {#if loading && nothingYet}
          <div class="invitation-skeleton" aria-hidden="true">
            <span class="skeleton-person"></span>
            <span></span>
            <span></span>
            <span class="skeleton-action"></span>
          </div>
          <p class="visually-hidden" role="status">Loading invitation</p>
        {:else if failure !== null}
          <p>{failure}</p>
          <button class="btn" onclick={() => void load(token)} disabled={loading}>
            {loading ? 'Trying again…' : 'Try again'}
          </button>
        {:else if invitation !== null}
          <div class="invited-user">
            <Avatar account={invitation.account} size={48} />
            <div>
              <strong>{invitation.account.display_name}</strong>
              <span class="mono dim">@{invitation.account.login}</span>
            </div>
            <Chip tone={statusTone(invitation.status)}
              >{invitation.status.slice(0, 1).toUpperCase() + invitation.status.slice(1)}</Chip
            >
          </div>

          <dl class="invitation-details">
            <div>
              <dt>Access</dt>
              <dd>{roleLabel(invitation)}</dd>
            </div>
            <div>
              <dt>Scope</dt>
              <dd>{invitation.target_name ?? 'Smyklot application'}</dd>
            </div>
            <div>
              <dt>Expires</dt>
              <dd>
                <time datetime={invitation.expires_at}>{formatDateTime(invitation.expires_at)}</time
                >
              </dd>
            </div>
            <div>
              <dt>Invited by</dt>
              <dd>@{invitation.created_by.login}</dd>
            </div>
          </dl>

          {#if invitation.status === 'pending'}
            <p>
              Continue with GitHub to prove you are @{invitation.account.login}, then accept or
              decline this invitation
            </p>
            <div class="invitation-actions">
              <a
                class="btn btn-signal"
                href={api.signInUrl({ token, action: 'accept' })}
                rel="nofollow"
              >
                Accept with GitHub
              </a>
              <a
                class="btn btn-quiet"
                href={api.signInUrl({ token, action: 'decline' })}
                rel="nofollow"
              >
                Decline
              </a>
            </div>
          {:else if invitation.status === 'accepted'}
            <p>This invitation was accepted</p>
            <a class="btn btn-signal" href={api.signInUrl()}>Open panel</a>
          {:else if invitation.status === 'declined'}
            <p>This invitation was declined</p>
          {:else if invitation.status === 'expired'}
            <p>This invitation expired. Ask the sender to reissue it</p>
          {:else}
            <p>This invitation was revoked</p>
          {/if}
        {/if}
      </div>
    </section>

    <PageFooter {build} />
  </div>
</main>

<style>
  /* Three rows, and the mark shares the top one with the empty bottom one. Both
     flexible rows take the same share, so the group between them keeps the exact
     centre it had before the mark moved above it - the mark grows into the space
     that was already there rather than pushing the card down. When the content
     outgrows the viewport the flexible rows collapse and the page scrolls from
     the top, so nothing lands above the scroll origin. */
  .invitation-shell {
    /* The title's own height plus the space under it - stated once, because the
       mark is centred against the card and has to discount what sits between. */
    --invitation-title-block: calc(1.0625rem * 1.3 + var(--space-3));

    display: grid;
    grid-template-rows: 1fr auto 1fr;
    max-width: 42rem;
    min-height: 100dvh;
    padding-block: var(--space-6);
    row-gap: var(--space-6);
  }

  /* Centred in the whitespace between the window's top edge and the card, so the
     mark holds the middle of that gap at any window height rather than drifting
     with a fixed offset. Its row ends at the title, not at the card, so it
     carries the title's block as padding above and centres the padded box - which
     puts the mark itself half that block lower, on the card's gap. The row gap is
     symmetric across both flexible rows, so none of this moves the card. */
  /* Stretched to fill its row rather than centred inside it, so the element's own
     height *is* the gap above the card. That is what the sky measures itself
     against; the mark still sits in the middle of it. */
  .invitation-brand {
    align-items: center;
    align-self: stretch;
    display: flex;
    justify-content: center;
    padding-top: var(--invitation-title-block);
    position: relative;
  }

  /* Sized against the gap it sits in, not in rem or `vh`. The page is centred, so
     that gap grows when the card is short and shrinks when it is tall - a sky
     with a fixed reach lands differently in each state, and whichever line it
     leaves inside its fade reads against a mid-tone. As a multiple of the gap,
     the title falls at the same point on the falloff every time and the footer
     stays past the end of it. */
  /* Fixed rather than absolute: the sky is deliberately larger than the window,
     and an absolutely positioned element that size extends the document's
     scrollable area - a page with nothing on it would gain scrollbars in both
     directions. A fixed one is outside that reckoning. The cost is that it is
     anchored to the window rather than to the mark, so `top` approximates where
     the mark sits; at this size a few pixels of drift is not visible. */
  .invitation-brand :global(.night-sky) {
    left: 50%;
    position: fixed;
    top: 16vh;
    translate: -50% -50%;
  }

  /* Everything outside the card stands on the sky, and the sky is night whichever
     theme the page is in, so this page writes in light ink in both. The card
     keeps the page's own palette: it is a panel laid on the sky, not part of it. */
  .invitation-brand :global(.mark-name) {
    color: rgb(246 249 255);
  }

  .invitation-brand :global(.mark-part) {
    color: rgb(186 203 233);
  }

  .invitation-shell :global(.foot-name) {
    color: rgb(214 226 246);
  }

  .invitation-shell :global(.foot-host) {
    color: rgb(160 180 214);
  }

  /* Reads as the card's own title from the outside, so it keeps the size the
     plate header gave it. */
  .invitation-title {
    color: rgb(246 249 255);
    font: 700 1.0625rem / 1.3 var(--sans);
    letter-spacing: 0;
    margin: 0 0 var(--space-3);
  }

  .invitation-card {
    border-color: var(--dialog-border);
    box-shadow: var(--shadow-plate);
    margin-bottom: 0;
  }

  .invitation-card.loading {
    cursor: progress;
  }

  .invitation-card :global(.plate-body) {
    padding: var(--space-5);
  }

  /* No rule above it: the card's own edge already separates the footer from the
     page's content, and a second line so close to it only crowds the corner. */
  .invitation-shell :global(.foot) {
    border-top: 0;
    margin-top: var(--space-4);
    padding-top: 0;
  }

  .invited-user {
    align-items: center;
    display: grid;
    gap: 0.75rem;
    grid-template-columns: auto minmax(0, 1fr) auto;
  }

  .invited-user > div {
    display: grid;
    gap: 0.15rem;
    min-width: 0;
  }

  .invitation-details {
    display: grid;
    gap: 0.75rem;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    margin: 1.25rem 0;
  }

  .invitation-details div {
    border-top: 1px solid var(--rule);
    display: grid;
    gap: 0.25rem;
    padding-top: 0.625rem;
  }

  dt {
    color: var(--dim);
    font: 650 var(--font-size-compact) / 1.3 var(--sans);
    letter-spacing: 0.02em;
  }

  dd {
    margin: 0;
  }

  .invitation-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
  }

  .invitation-skeleton {
    display: grid;
    gap: var(--space-3);
  }

  .invitation-skeleton span {
    animation: invitation-skeleton-pulse 1.35s ease-in-out infinite alternate;
    background: var(--surface-inset);
    border-radius: var(--radius-control);
    display: block;
    height: 0.875rem;
    width: 72%;
  }

  .invitation-skeleton .skeleton-person {
    height: 3rem;
    width: 100%;
  }

  .invitation-skeleton .skeleton-action {
    height: var(--control-height);
    margin-top: var(--space-2);
    width: 9rem;
  }

  @keyframes invitation-skeleton-pulse {
    from {
      opacity: 0.5;
    }

    to {
      opacity: 0.9;
    }
  }

  @media (max-width: 36rem) {
    .invitation-details {
      grid-template-columns: minmax(0, 1fr);
    }

    .invited-user {
      grid-template-columns: auto minmax(0, 1fr);
    }

    .invited-user :global(.chip) {
      grid-column: 1 / -1;
      justify-self: start;
    }
  }
</style>

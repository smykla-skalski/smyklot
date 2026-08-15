<script lang="ts">
  import { PanelApiError, type PanelApi } from '../lib/api';
  import type { PanelBuild } from '../lib/base';
  import { formatDateTime } from '../lib/format';
  import {
    applyDocumentTheme,
    DEFAULT_THEME_DISPLAY,
    isThemeDisplay,
    resolveThemeDisplay,
    systemThemeDisplay,
    type ThemeDisplay,
  } from '../lib/preferences';
  import { createPrefsSync } from '../lib/preferences-sync';
  import type { PanelInvitation } from '../lib/types';
  import Avatar from './Avatar.svelte';
  import BrandMark from './BrandMark.svelte';
  import Chip, { type ChipTone } from './Chip.svelte';
  import NightSky from './NightSky.svelte';
  import PageFooter from './PageFooter.svelte';
  import ThemeSwitch from './ThemeSwitch.svelte';

  const { api, token, build }: { api: PanelApi; token: string; build: PanelBuild } = $props();

  /* One source for the mark's size: the component needs the number, and the sky
     needs it in CSS to find the middle of the mark it opens out from. */
  const MARK_SIZE = 104;

  /* The same synced document the panel writes, without the stream behind it: a
     write here stays pending in local storage and goes up on the first connect
     after signing in, so the theme chosen on this page is the one that greets
     the reader inside. */
  const prefs = createPrefsSync();

  /* Read once and never watched. The page opens on whatever the system asks for,
     but it opens on it as a choice already made - the switch shows light or dark
     picked from the first paint, and the page holds it. A `MediaQuery` here would
     repaint the page under a reader who is midway through an invitation because
     their laptop reached sunset, and this is the one page with no account behind
     it to remember what they would rather have. */
  const systemAtOpen = systemThemeDisplay();

  let theme = $state<ThemeDisplay>(storedTheme());
  const resolvedTheme = $derived(resolveThemeDisplay(theme, systemAtOpen));

  /* A link that names no invitation is a different answer from a request that did
     not get through, and the two want opposite things from the reader: one is
     over, the other is worth pressing again. They are one field rather than two so
     they cannot disagree about which of them is showing. */
  type InvitationFailure = { missing: true } | { missing: false; message: string };

  let invitation = $state<PanelInvitation | null>(null);
  let loading = $state(true);
  let failure = $state<InvitationFailure | null>(null);

  /* The skeleton stands in for an answer the page does not have yet. Once it has
     one, a retry keeps it on screen and marks the card busy: swapping it back to
     a placeholder of a different height moves the whole centred stack, twice. */
  const nothingYet = $derived(invitation === null && failure === null);

  /* The card carries no header of its own, so its title stands above it and
     names whichever of the three states the card is showing. It follows what is
     displayed rather than the request, so a retry does not flicker it. */
  const title = $derived(
    invitation !== null
      ? 'Access invitation'
      : failure === null
        ? 'Invitation'
        : failure.missing
          ? 'Not found'
          : 'Invitation unavailable',
  );

  $effect(() => {
    void load(token);
  });

  $effect(() => {
    applyDocumentTheme(document, resolvedTheme);
  });

  function storedTheme(): ThemeDisplay {
    const value = prefs.get('theme');
    return typeof value === 'string' && isThemeDisplay(value) ? value : DEFAULT_THEME_DISPLAY;
  }

  function selectTheme(nextTheme: ThemeDisplay): void {
    theme = nextTheme;
    prefs.set('theme', nextTheme);
  }

  async function load(requestedToken: string): Promise<void> {
    loading = true;
    try {
      invitation = await api.fetchInvitation(requestedToken);
      failure = null;
    } catch (error) {
      failure =
        error instanceof PanelApiError && error.status === 404
          ? { missing: true }
          : { missing: false, message: error instanceof Error ? error.message : String(error) };
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

  /* Built from the login rather than taken from the API, which does not carry a
     profile URL. `encodeURIComponent` because a login reaches this page from a
     token a stranger supplied: GitHub logins cannot contain anything that needs
     escaping, but the guarantee is GitHub's rather than this page's. */
  function githubProfile(login: string): string {
    return `https://github.com/${encodeURIComponent(login)}`;
  }

  /* What the offer covers, as the tail of a sentence. The kind carries its weight:
     "for Smykla Skalski" leaves a reader guessing whether that is a company or a
     person, and "for the Smykla Skalski organization" does not. A system-role
     offer has no target and ends the sentence where it stands. */
  function scopePhrase(value: PanelInvitation): string {
    if (value.target_name === undefined) return '';
    if (value.target_kind === undefined) return ` for ${value.target_name}`;
    return ` for the ${value.target_name} ${value.target_kind.toLowerCase()}`;
  }

  function roleLabel(value: PanelInvitation): string {
    if (value.system_role === 'root') return 'Root';
    const role = value.role ?? 'viewer';
    return role.slice(0, 1).toUpperCase() + role.slice(1);
  }
</script>

<svelte:head>
  <title>Access Invitation | SMYKLOT</title>
</svelte:head>

<main class="shell invitation-shell">
  <div class="invitation-brand" style="--invitation-mark-size: {MARK_SIZE}px">
    <NightSky />
    <BrandMark stacked size={MARK_SIZE} />
  </div>

  <div class="invitation-main">
    <div class="invitation-head">
      <h1 class="invitation-title" id="invitation-title">{title}</h1>
      <ThemeSwitch
        name="invitation-theme"
        theme={resolvedTheme}
        surface="night"
        system={false}
        onSelect={selectTheme}
      />
    </div>

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
        {:else if failure !== null && failure.missing}
          <div class="invitation-missing">
            <p class="missing-code" aria-hidden="true">404</p>
            <p class="missing-lead">This invitation link does not exist</p>
            <p class="missing-note">
              It may have been withdrawn, or the address may be incomplete. Ask whoever sent it for
              a new one
            </p>
          </div>
        {:else if failure !== null}
          <p>{failure.message}</p>
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
              <dt>Your role</dt>
              <dd>{roleLabel(invitation)}</dd>
            </div>
            <div>
              <dt>Applies to</dt>
              <dd class="invitation-scope">
                {#if invitation.target_login === undefined}
                  <span>{invitation.target_name ?? 'Smyklot application'}</span>
                {:else}
                  <a
                    class="link"
                    href={githubProfile(invitation.target_login)}
                    target="_blank"
                    rel="noreferrer"
                  >
                    {invitation.target_name ?? invitation.target_login}
                  </a>
                {/if}
                {#if invitation.target_kind !== undefined}
                  <span class="scope-kind">{invitation.target_kind}</span>
                {/if}
              </dd>
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
              <dd>
                <a
                  class="link"
                  href={githubProfile(invitation.created_by.login)}
                  target="_blank"
                  rel="noreferrer"
                >
                  @{invitation.created_by.login}
                </a>
              </dd>
            </div>
          </dl>

          {#if invitation.status === 'pending'}
            <p class="invitation-consent">
              Accepting gives you {roleLabel(invitation)} access to Smyklot, the bot that approves and
              merges pull requests{scopePhrase(invitation)}
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
    /* Smaller than the panel's own compact control. There is one of these on the
       whole page and it is not what the reader came for, so it steps back from
       the title it shares a row with rather than matching it. */
    --invitation-switch-height: 1.75rem;

    display: grid;
    grid-template-rows: 1fr auto 1fr;
    max-width: 42rem;
    min-height: 100dvh;
    padding-block: var(--space-6);
    row-gap: var(--space-6);
  }

  /* Stretched to fill its row rather than centred inside it, so the element's own
     height *is* the whitespace above the page's content. That is what the sky
     measures itself against, and the mark sits in the middle of it.

     Nothing is discounted from that middle. It used to carry the head row's
     height as padding, which centred the mark on the gap up to the *card* and so
     left it half that padding low against the whitespace a reader actually sees -
     the row of title and switch reads as the card's own head, not as part of the
     space above it. The mark is one object, icon and wordmark together, and it is
     the object that gets centred: with the padding there the icon looked about
     right and SMYKLOT hung below the middle, which is what gave it away. */
  .invitation-brand {
    align-items: center;
    align-self: stretch;
    display: flex;
    justify-content: center;
    position: relative;
  }

  /* Sized against the gap it sits in, not in rem or `vh`. The page is centred, so
     that gap grows when the card is short and shrinks when it is tall - a sky
     with a fixed reach lands differently in each state, and whichever line it
     leaves inside its fade reads against a mid-tone. As a multiple of the gap,
     the title falls at the same point on the falloff every time and the footer
     stays past the end of it. */
  /* Centred on the mark, and its height is read from this row - which is the gap
     above the card. The page is centred, so that gap grows when the card is short
     and shrinks when it is tall; a sky measured in rem or `vh` lands differently
     in each state, and whichever line it leaves inside its fade reads against a
     mid-tone. As a multiple of the gap, the title sits at the same point on the
     falloff every time. */
  .invitation-brand :global(.night-sky) {
    left: 50%;
    top: 50%;
    translate: -50% -50%;
  }

  /* The sky is a viewport wide and centred on a column that is narrower, so it
     reaches the window's edges - and past them once a scrollbar takes a slice out
     of the content box. Clipped rather than hidden, which would make a scroll
     container of the page, and scoped to this page by what it contains so the
     panel's own horizontal scrollers are left alone. */
  :global(html:has(.invitation-shell)) {
    overflow-x: clip;
  }

  /* Everything outside the card stands on the sky, and the sky is night whichever
     theme the page is in, so this page writes in light ink in both. The card
     keeps the page's own palette: it is a panel laid on the sky, not part of it. */
  .invitation-brand :global(.mark-name) {
    color: rgb(246 249 255);
  }

  /* The card's own head, lifted out of it: the title on the left names whichever
     state the card is showing, and the switch on the right is the one control on
     the page that is not part of the invitation. The row keeps the control's
     height whatever the title does, so the gap the mark measures itself against
     does not move when the title wraps. */
  .invitation-head {
    align-items: center;
    display: flex;
    gap: var(--space-3);
    justify-content: space-between;
    margin-bottom: var(--space-3);
    min-height: var(--invitation-switch-height);
  }

  /* The control reads its own height from this, so the row and the control cannot
     disagree about how tall the head is. */
  .invitation-head :global(fieldset) {
    --local-control-height: var(--invitation-switch-height);
  }

  /* Reads as the card's own title from the outside, so it keeps the size the
     plate header gave it. */
  .invitation-title {
    color: rgb(246 249 255);
    font: 700 1.0625rem / 1.3 var(--sans);
    letter-spacing: 0;
    margin: 0;
    min-width: 0;
  }

  /* A floor under the card, so the three states are not three different page
     layouts. It stops the stack resettling when a load finishes, and it keeps the
     gap above the card - which is what the sky measures itself against - within a
     narrow range instead of doubling when the card holds one line of error. */
  /* The one thing on the page that is not the sky, so it is not quite opaque
     either: the sky reads through it and the card sits *in* the scene rather than
     on top of it. The blur behind is what makes that safe - it takes the stars out
     of the ground the text stands on, leaving an even wash instead of specks of
     white under the type.

     The lift is what pays for the rest. Straight translucency costs contrast,
     because the sky is denser at the top of the card than at the bottom and the
     type at the top ends up standing on the darkest ground: at 92% opaque and no
     lift, `dt` - the dimmest type on the card - fell to 4.81:1 against a 4.5
     floor, and that was already as far as it could go. Brightening the backdrop
     before it shows through separates the two: the card transmits the sky's
     *shape* without transmitting its darkness, so it can be a great deal more
     see-through and read better while doing it. Measured on the light page across
     620-1600px window heights - see the commit for the numbers.

     The dark page lifts the other way. Its surface is already close to the sky, so
     brightening the backdrop would erase the difference the effect is made of;
     dropping it instead makes the sky behind the card read as depth. */
  .invitation-card {
    --invitation-card-lift: 1.6;

    backdrop-filter: blur(22px) saturate(1.4) brightness(var(--invitation-card-lift));
    background: color-mix(in srgb, var(--strip) 86%, transparent);
    border-color: var(--dialog-border);
    box-shadow: var(--shadow-plate);
    margin-bottom: 0;
    min-height: 19rem;
  }

  :global(:root[data-theme='dark']) .invitation-card {
    --invitation-card-lift: 0.72;
  }

  .invitation-card :global(.plate-body) {
    align-content: center;
    display: grid;
    min-height: inherit;
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

  /* The panel has no links to speak of - it is built from buttons - so this is the
     first place one has to look like it belongs. It borrows the action ink rather
     than inventing a link colour, and the underline is drawn faintly at rest and
     fully on hover, so a page with two of them close together does not read as
     ruled. Press is a colour step, not the system's usual shrink: scaling a run of
     text inside a sentence moves the words around it. */
  .link {
    border-radius: 2px;
    color: var(--brand-action-text);
    font-weight: 600;
    text-decoration: underline;
    text-decoration-color: color-mix(in srgb, currentcolor 35%, transparent);
    text-decoration-thickness: 1px;
    text-underline-offset: 0.18em;
    transition:
      color var(--duration-fast) var(--ease-standard),
      text-decoration-color var(--duration-fast) var(--ease-standard);
  }

  .link:hover {
    text-decoration-color: currentcolor;
  }

  .link:active {
    color: color-mix(in srgb, var(--brand-action-text) 78%, var(--text-primary));
  }

  .link:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: 2px;
  }

  .invitation-scope {
    align-items: baseline;
    column-gap: 0.4rem;
    display: flex;
    flex-wrap: wrap;
  }

  /* Whether accepting joins an organisation or one person's installation. Quiet,
     because it qualifies the name rather than competing with it. */
  .scope-kind {
    color: var(--text-muted);
    font: 600 var(--font-size-meta) / 1.2 var(--sans);
  }

  /* A link that names no invitation is not a failed request, so it is not offered
     a retry: there is nothing on the other end to try again for. It says so as a
     404 because that is the one thing every reader already recognises, and then in
     words, because the number alone does not say what to do next. */
  .invitation-missing {
    display: grid;
    gap: var(--space-2);
    justify-items: center;
    text-align: center;
  }

  .missing-code {
    color: var(--text-muted);
    font: 800 3.25rem / 1 var(--sans);
    letter-spacing: 0.06em;
    margin: 0;
    opacity: 0.55;
  }

  .missing-lead {
    color: var(--text-primary);
    font: 650 1.0625rem / 1.35 var(--sans);
    margin: 0;
  }

  .missing-note {
    color: var(--text-secondary);
    margin: 0;
    max-width: 28rem;
  }

  /* What the reader is actually being asked to consent to, so it is ruled off from
     the invitation's facts above it rather than reading as one more of them. Three
     short sentences, in this order because that is the order the questions arrive
     in: what the buttons do, why they do it, and what it costs. The last one is
     the one that gets the click - the panel signs in through a scopeless OAuth
     App, so "public profile only" is exactly true and worth saying out loud on a
     page asking a stranger to authorise something. Keep it true if that changes
     (`newGitHubSignIn` in internal/panel/github.go). */
  .invitation-consent {
    border-top: 1px solid var(--rule);
    color: var(--text-secondary);
    padding-top: 0.875rem;
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

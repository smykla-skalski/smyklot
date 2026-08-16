<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { PanelApiError, type PanelApi } from '../api';
  import { panelUrl, type PanelBuild } from '../base';
  import { formatDateTime } from '../format';
  import { describeFailure } from '../panel-error';
  import type { PanelInvitation } from '../types';
  import Avatar from './Avatar.svelte';
  import Chip, { type ChipTone } from './Chip.svelte';
  import ErrorCard from './ErrorCard.svelte';
  import NightPage from './NightPage.svelte';

  const {
    api,
    base,
    token,
    build,
  }: { api: PanelApi; base: string; token: string; build: PanelBuild } = $props();

  /* A token that names nothing is answered the way every other dead address is.
     It is a 404 and it says so in the same words, without naming invitations: a
     reader holding a link that leads nowhere cannot act on being told what the
     address would have been for, and saying it only describes something they have
     no way to reach. */
  const missingContent = describeFailure({ status: 404, code: 'not_found', message: '' });

  /* A link that names no invitation is a different answer from a request that did
     not get through, and the two want opposite things from the reader: one is
     over, the other is worth pressing again. They are one field rather than two so
     they cannot disagree about which of them is showing. */
  type InvitationFailure = { missing: true } | { missing: false; message: string };

  const invitationQuery = createQuery(() => ({
    queryKey: ['invitation', token],
    queryFn: () => api.fetchInvitation(token),
    enabled: token !== '',
    retry: (failureCount, error) =>
      !(error instanceof PanelApiError && error.status === 404) && failureCount < 1,
  }));
  const invitation = $derived<PanelInvitation | null>(invitationQuery.data ?? null);
  const loading = $derived(invitationQuery.isFetching);
  const failure = $derived<InvitationFailure | null>(invitationFailure(invitationQuery.error));

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
          ? missingContent.title
          : 'Invitation unavailable',
  );

  function invitationFailure(error: unknown): InvitationFailure | null {
    if (error === null) return null;
    return error instanceof PanelApiError && error.status === 404
      ? { missing: true }
      : { missing: false, message: error instanceof Error ? error.message : String(error) };
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

<!-- The tab says "Access Invitation" for every state the page has an invitation
     for, and names the error otherwise: a 404 that leads nowhere should not be
     announcing an invitation in the one place a reader keeps it. -->
<NightPage
  {title}
  documentTitle={failure?.missing === true ? missingContent.title : 'Access Invitation'}
  {build}
  busy={loading}
>
  {#if loading && nothingYet}
    <div class="invitation-skeleton" aria-hidden="true">
      <span class="skeleton-person"></span>
      <span></span>
      <span></span>
      <span class="skeleton-action"></span>
    </div>
    <p class="visually-hidden" role="status">Loading invitation</p>
  {:else if failure !== null && failure.missing}
    <ErrorCard
      content={missingContent}
      panelHref={panelUrl(base, '/')}
      signInHref={api.signInUrl()}
    />
  {:else if failure !== null}
    <p>{failure.message}</p>
    <button class="btn" onclick={() => void invitationQuery.refetch()} disabled={loading}>
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
          <time datetime={invitation.expires_at}>{formatDateTime(invitation.expires_at)}</time>
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
        Accepting gives you {roleLabel(invitation)} access to Smyklot, the bot that approves and merges
        pull requests{scopePhrase(invitation)}
      </p>
      <div class="invitation-actions">
        <a class="btn btn-signal" href={api.signInUrl({ token, action: 'accept' })} rel="nofollow">
          Accept with GitHub
        </a>
        <a class="btn btn-quiet" href={api.signInUrl({ token, action: 'decline' })} rel="nofollow">
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
</NightPage>

<style>
  /* The page's shell, sky, card and theme switch live in NightPage - this page
     and the error pages are the same page with different contents in the card. */
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

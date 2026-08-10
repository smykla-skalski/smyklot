<script lang="ts">
  import type { PanelApi } from '../lib/api';
  import type { PanelBuild } from '../lib/base';
  import { formatDateTime } from '../lib/format';
  import type { PanelInvitation } from '../lib/types';
  import Avatar from './Avatar.svelte';
  import Chip, { type ChipTone } from './Chip.svelte';
  import PageFooter from './PageFooter.svelte';
  import Plate from './Plate.svelte';

  const {
    api,
    token,
    iconUrl,
    build,
  }: { api: PanelApi; token: string; iconUrl: string; build: PanelBuild } = $props();

  let invitation = $state<PanelInvitation | null>(null);
  let loading = $state(true);
  let failure = $state<string | null>(null);

  $effect(() => {
    void load(token);
  });

  async function load(requestedToken: string): Promise<void> {
    loading = true;
    failure = null;
    try {
      invitation = await api.fetchInvitation(requestedToken);
    } catch (error) {
      failure = error instanceof Error ? error.message : String(error);
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

<main class="shell invitation-shell">
  <header class="invitation-brand">
    <img src={iconUrl} width="42" height="42" alt="" />
    <div>
      <strong>Smyklot</strong>
      <span>Panel invitation</span>
    </div>
  </header>

  {#if loading}
    <Plate label="Invitation" tone="lead">
      <div class="invitation-skeleton" aria-hidden="true">
        <span class="skeleton-person"></span>
        <span></span>
        <span></span>
        <span class="skeleton-action"></span>
      </div>
      <p class="visually-hidden" role="status">Loading invitation</p>
    </Plate>
  {:else if failure !== null}
    <Plate label="Invitation unavailable" tone="alarm">
      <p>{failure}</p>
      <button class="btn" onclick={() => void load(token)}>Try again</button>
    </Plate>
  {:else if invitation !== null}
    <Plate label="GitHub access invitation" tone="lead">
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
            <time datetime={invitation.expires_at}>{formatDateTime(invitation.expires_at)}</time>
          </dd>
        </div>
        <div>
          <dt>Invited by</dt>
          <dd>@{invitation.created_by.login}</dd>
        </div>
      </dl>

      {#if invitation.status === 'pending'}
        <p>
          Continue with GitHub to prove you are @{invitation.account.login}, then accept or decline
          this invitation
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
    </Plate>
  {/if}

  <PageFooter {build} />
</main>

<style>
  .invitation-shell {
    display: grid;
    grid-template-rows: auto 1fr auto;
    max-width: 42rem;
    min-height: 100dvh;
    padding-block: clamp(var(--space-6), 7vh, 4.5rem) var(--space-6);
  }

  .invitation-brand {
    align-items: center;
    display: flex;
    gap: 0.75rem;
    margin-bottom: 1rem;
  }

  .invitation-brand img {
    border-radius: var(--r-ctl);
  }

  .invitation-brand div {
    display: grid;
    gap: 0.05rem;
  }

  .invitation-brand span {
    color: var(--dim);
    font-size: var(--font-size-meta);
  }

  .invitation-brand strong {
    font-size: var(--font-size-title);
    font-weight: 700;
  }

  .invitation-shell :global(.plate) {
    align-self: center;
    border-color: var(--dialog-border);
    box-shadow: var(--shadow-plate);
    margin-block: var(--space-6);
  }

  .invitation-shell :global(.plate-head) {
    min-height: 3.25rem;
  }

  .invitation-shell :global(.plate-body) {
    padding: var(--space-5);
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

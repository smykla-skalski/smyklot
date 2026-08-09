<script lang="ts">
  import type { PanelApi } from '../lib/api';
  import type { PanelBuild } from '../lib/base';
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

  function roleLabel(role: PanelInvitation['role']): string {
    return role.slice(0, 1).toUpperCase() + role.slice(1);
  }
</script>

<main class="shell invitation-shell">
  <header class="invitation-brand">
    <img src={iconUrl} width="42" height="42" alt="" />
    <div>
      <span>SMYKLOT</span>
      <strong>PANEL INVITATION</strong>
    </div>
  </header>

  {#if loading}
    <Plate label="Invitation" tone="lead">
      <p class="dim">Reading the invitation…</p>
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
        <Chip tone={statusTone(invitation.status)}>{invitation.status.toUpperCase()}</Chip>
      </div>

      <dl class="invitation-details">
        <div>
          <dt>ACCESS</dt>
          <dd>{roleLabel(invitation.role)}</dd>
        </div>
        <div>
          <dt>SCOPE</dt>
          <dd>{invitation.target_name ?? 'All installations'}</dd>
        </div>
        <div>
          <dt>EXPIRES</dt>
          <dd>
            <time datetime={invitation.expires_at}
              >{new Date(invitation.expires_at).toLocaleString()}</time
            >
          </dd>
        </div>
        <div>
          <dt>INVITED BY</dt>
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
        <p>This invitation expired — ask the sender to reissue it</p>
      {:else}
        <p>This invitation was revoked</p>
      {/if}
    </Plate>
  {/if}

  <PageFooter {build} />
</main>

<style>
  .invitation-shell {
    max-width: 44rem;
  }

  .invitation-brand {
    align-items: center;
    display: flex;
    gap: 0.75rem;
    margin-bottom: 1rem;
  }

  .invitation-brand img {
    border-radius: var(--radius-control);
  }

  .invitation-brand div {
    display: grid;
    gap: 0.05rem;
  }

  .invitation-brand span,
  .invitation-brand strong {
    font-family: var(--mono);
    letter-spacing: 0.1em;
  }

  .invitation-brand span {
    color: var(--dim);
    font-size: 0.625rem;
  }

  .invitation-brand strong {
    font-size: 0.875rem;
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
    border-top: 1px solid var(--line);
    display: grid;
    gap: 0.25rem;
    padding-top: 0.625rem;
  }

  dt {
    color: var(--dim);
    font: 600 0.625rem/1.3 var(--mono);
    letter-spacing: 0.11em;
  }

  dd {
    margin: 0;
  }

  .invitation-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
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

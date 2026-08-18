<script lang="ts">
  /**
   * The panel's front door.
   *
   * It stands on the same shell as an invitation and the error pages, because it
   * is the third page a reader can be on without a session, and the three of them
   * being one product matters more here than anywhere: this is where someone who
   * has just installed Smyklot arrives, and where everyone who ever signs out
   * comes back to.
   *
   * The card is small on purpose. There is one thing to do here, and a card sized
   * for an invitation's four rows of facts would leave a sentence and a button
   * floating in it.
   */
  import type { PanelApi } from '../api';
  import type { PanelBuild } from '../base';
  import { describeSessionEnd, type SessionEnded } from '../panel-session';
  import Button from './Button.svelte';
  import NightPage from './NightPage.svelte';

  const {
    api,
    build,
    ended = null,
  }: {
    api: PanelApi;
    build: PanelBuild;
    /** Set when a session ended while someone was using it, rather than never having one. */
    ended?: SessionEnded | null;
  } = $props();

  const notice = $derived(ended === null ? null : describeSessionEnd(ended));
  const title = $derived(notice?.title ?? 'Sign in');
  const offersSignIn = $derived(notice === null || notice.offersSignIn);
</script>

<NightPage {title} documentTitle={title} {build} size="compact">
  <div class="sign-in-body">
    {#if notice === null}
      <!-- Names the place and stops. It used to explain what Smyklot does and then
           list what the panel is for, which is a paragraph that goes stale the
           first time a feature is added and that nobody standing at a sign-in
           screen is reading anyway. -->
      <p class="sign-in-lead">The Smyklot control panel</p>
      <p class="sign-in-note">You need to be signed in to open it</p>
    {:else}
      <p class="sign-in-lead">{notice.lead}</p>
      <p class="sign-in-note">{notice.note}</p>
    {/if}

    {#if offersSignIn}
      <p class="sign-in-action">
        <Button tone="signal" href={api.signInUrl()} rel="nofollow">Sign in with GitHub</Button>
      </p>

      <!-- The one thing worth saying on a page whose only button hands someone to
           an OAuth consent screen, and the question that stops people using it: a
           bot that merges code asking to sign in reads as asking for write access.
           It is not. The panel signs in through a scopeless classic OAuth App, so
           GitHub offers public profile read alone, and the token is used for one
           `GET /user` and then dropped. Keep this true if that changes, in
           `newGitHubSignIn` in internal/panel/github.go. -->
      <p class="sign-in-consent">GitHub is asked for your public profile and nothing else</p>
    {/if}
  </div>
</NightPage>

<style>
  .sign-in-body {
    display: grid;
    gap: var(--space-2);
  }

  .sign-in-lead {
    color: var(--text-primary);
    font: 650 1.0625rem / 1.35 var(--sans);
    margin: 0;
  }

  .sign-in-note {
    color: var(--text-secondary);
    margin: 0;
  }

  /* A flex row rather than a line box, so the button's own height is the row's and
     the card's bottom padding is the gap a reader sees under it. */
  .sign-in-action {
    display: flex;
    margin: var(--space-4) 0 0;
  }

  /* Ruled off from the offer above it, the way the invitation rules off what
     accepting costs from the facts it is deciding on. Quiet, because it answers a
     question rather than making one. */
  .sign-in-consent {
    border-top: 1px solid var(--rule);
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    margin: var(--space-4) 0 0;
    padding-top: var(--space-3);
  }
</style>

<script lang="ts">
  import { resolve } from '$app/paths';

  import type { PanelApi } from '../api';
  import type { PanelBuild } from '../base';
  import { describeFailure, type PanelFailure } from '../panel-error';
  import { describeSessionEnd, type SessionEnded } from '../panel-session';
  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import NightPage from './NightPage.svelte';

  const {
    api,
    build,
    ended = null,
    failed = null,
    returnTo = null,
  }: {
    api: PanelApi;
    build: PanelBuild;
    /** Set when a session ended while someone was using it, rather than never having one. */
    ended?: SessionEnded | null;
    /** Set when a sign-in was attempted and did not finish. */
    failed?: PanelFailure | null;
    /** The address to come back to, when it is not the panel's front page. */
    returnTo?: string | null;
  } = $props();

  const notice = $derived(ended === null ? null : describeSessionEnd(ended));
  const title = $derived(notice?.title ?? 'Sign in');
  const offersSignIn = $derived(notice === null || notice.offersSignIn);
  /* The same table the error PAGES read, keyed by the same status and code. A
     sign-in failure is worded once, wherever it is shown. */
  const failure = $derived(failed === null ? null : describeFailure(failed));
  const href = $derived(api.signInUrl(undefined, returnTo ?? undefined));
</script>

<!--
@component
The panel's front door.

It stands on the same shell as an invitation and the error pages, because it
is the third page a reader can be on without a session, and the three of them
being one product matters more here than anywhere: this is where someone who
has just installed Smyklot arrives, and where everyone who ever signs out
comes back to.

The card is small on purpose. There is one thing to do here, and a card sized
for an invitation's four rows of facts would leave a sentence and a button
floating in it.

EVERY WAY IN LANDS HERE, including the addresses this panel does not have: a
reader with no session is shown this card whatever they asked for, so that the
route table cannot be read off which addresses answer. Which is also why the
card has to say where it is going to put them, and why that promise has to be
true - see `safeReturnPath` in internal/panel/auth.go for what it will and will
not honour.
-->

<NightPage {title} documentTitle={title} {build} size="compact">
  <div class="sign-in-body">
    {#if notice === null}
      <!-- THE FRONT DOOR SAYS WHAT THE PRODUCT DOES. This named the place and stopped,
           on the reasoning that nobody standing at a sign-in screen reads a paragraph -
           but the people who arrive here are the ones who have just installed Smyklot
           and the ones who followed a link to it, and for both of them the sentence
           existed only on the invitation page. One sentence, about what it does rather
           than what the panel contains, so a feature cannot make it stale. -->
      <p class="sign-in-lead">Smyklot</p>
      <p class="sign-in-note">
        Approves and merges pull requests for your GitHub organization or account, driven by slash
        commands and the owners file you already keep
      </p>
    {:else}
      <p class="sign-in-lead">{notice.lead}</p>
      <p class="sign-in-note">{notice.note}</p>
    {/if}

    <!-- A sign-in that failed is answered where signing in happens, so the reason
         and the button that retries it are one thing. It used to be a page of its
         own, reached by a redirect, with the way back a link that started the flow
         from the top. -->
    {#if failure !== null}
      <p class="sign-in-error" role="status">
        <Icon name="alert" size="sm" />
        <span>{failure.lead}</span>
      </p>
    {/if}

    {#if offersSignIn}
      <p class="sign-in-action">
        <Button tone="signal" {href} rel="nofollow">Sign in with GitHub</Button>
      </p>

      <!-- The one thing worth saying on a page whose only button hands someone to
           an OAuth consent screen, and the question that stops people using it: a
           bot that merges code asking to sign in reads as asking for write access.
           It is not. The panel signs in through a scopeless classic OAuth App, so
           GitHub offers public profile read alone, and the token is used for one
           `GET /user` and then dropped. Keep this true if that changes, in
           `newGitHubSignIn` in internal/panel/github.go.

           AND WHERE IT PUTS YOU, which is the half that was a lie. The card said
           "you come back here afterwards" while the server sent everybody to the
           front page, so a pasted link to one workspace's plan was worth nothing
           once you had signed in. It says which page now, because a promise that
           names its destination is one somebody notices when it breaks. -->
      <p class="sign-in-consent">
        GitHub is asked for your public profile and nothing else, and you come back to
        {returnTo === null ? 'the panel' : 'this page'} afterwards
      </p>
    {/if}

    <!-- The three a front door owes a stranger: what is done with their data,
         what they are agreeing to, and how to reach a person. Resolved rather
         than concatenated, like every other address here, so removing either page
         stops compiling instead of shipping a dead link on the front door. -->
    <nav class="sign-in-links" aria-label="Legal and help">
      <a href={resolve('/privacy')}>Privacy</a>
      <a href={resolve('/terms')}>Terms</a>
      <a href="mailto:support@smyklot.com">Support</a>
    </nav>
  </div>
</NightPage>

<style>
  .sign-in-body {
    display: grid;
    gap: var(--space-2);
  }

  .sign-in-lead {
    color: var(--text-primary);
    font: 650 1.0625rem / var(--leading-body) var(--sans);
    margin: 0;
  }

  .sign-in-note {
    color: var(--text-secondary);
    margin: 0;
  }

  /* Its own tint rather than a Callout: this sits inside a card on the night sky,
     where a Callout's full surface reads as a second card. */
  .sign-in-error {
    align-items: center;
    background: var(--danger-tint);
    border-radius: var(--r-ctl);
    color: var(--danger);
    display: flex;
    font-size: var(--font-size-compact);
    gap: var(--space-2);
    margin: var(--space-3) 0 0;
    padding: var(--space-2) var(--space-3);
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
    border-top: 1px solid var(--border-subtle);
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    margin: var(--space-4) 0 0;
    padding-top: var(--space-3);
  }

  .sign-in-links {
    color: var(--text-muted);
    display: flex;
    flex-wrap: wrap;
    font-size: var(--font-size-compact);
    gap: var(--space-4);
    margin-block-start: var(--space-3);
  }

  /* The target is the row rather than the words: three links on one line at this
     size are otherwise 14px tall, which is under 2.5.8 whatever the spacing. */
  .sign-in-links a {
    align-items: center;
    color: var(--text-secondary);
    display: inline-flex;
    min-block-size: var(--field-target-min);
    text-decoration: none;
  }

  .sign-in-links a:hover {
    color: var(--text-primary);
    text-decoration: underline;
  }
</style>

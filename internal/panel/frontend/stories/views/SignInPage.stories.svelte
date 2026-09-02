<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import SignInPage from '#lib/components/SignInPage.svelte';
  import { stubApi } from '../support/api.js';

  const BUILD = { version: '1.37.0', serviceHost: 'smyklot.com' };

  const { Story } = defineMeta({
    title: 'Views/SignInPage',
    component: SignInPage,
    args: { api: stubApi(), build: BUILD },
  });
</script>

<!--
  The one page a stranger sees. Its only button hands them to an OAuth consent screen,
  so the card also answers the question that stops people using it: a bot that merges
  code asking to sign in reads as asking for write access, and it is not - the panel
  signs in through a scopeless classic OAuth App.
-->
<Story name="Signed out" />

<!-- A session that ended says so, rather than looking like a fresh arrival. -->
<Story name="Session ended" args={{ ended: { code: 'expired', reason: '' } }} />

<!--
  A sign-in that did not finish, answered where signing in happens. This used to be a
  page of its own, which put the reason and the button that retries it on different
  screens. The words come from the same table the error pages read, keyed by the status
  and code the server redirected back with.
-->
<Story
  name="Sign-in failed"
  args={{ failed: { status: 401, code: 'sign_in_failed', message: '' } }}
/>

<!--
  Arrived at a deep address. The card promises to put the reader back on it, which is a
  promise the server keeps - and one it used to break, sending everybody to the front
  page while this line said otherwise.
-->
<Story name="Returning to a page" args={{ returnTo: '/workspace/acme/sync/plan' }} />

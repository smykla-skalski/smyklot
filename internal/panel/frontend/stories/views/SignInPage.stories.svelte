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

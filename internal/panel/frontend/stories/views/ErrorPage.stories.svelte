<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import ErrorPage from '#lib/components/ErrorPage.svelte';
  import { stubApi } from '../support/api.js';

  const BUILD = { version: '1.37.0', serviceHost: 'smyklot.com' };
  const failure = (status: number, code: string) => ({ status, code, message: '' });

  const { Story } = defineMeta({
    title: 'Views/ErrorPage',
    component: ErrorPage,
    args: { api: stubApi(), base: '', build: BUILD, failure: failure(404, 'not_found') },
  });
</script>

<!--
  A server error rendered as a page rather than as a toast over an empty panel: the
  status reaches the browser in a meta tag, so the page that boots already knows what
  happened without a second request.

  The number carries no accent - it is the one thing every reader recognises, and
  colouring it would make it a state rather than a fact.
-->
<Story name="Not found" />

<Story name="Sign-in stopped" args={{ failure: failure(401, 'sign_in_failed') }} />

<Story name="Forbidden" args={{ failure: failure(403, 'forbidden') }} />

<Story name="Service error" args={{ failure: failure(500, 'internal') }} />

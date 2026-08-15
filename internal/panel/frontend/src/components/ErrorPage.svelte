<script lang="ts">
  /**
   * What the panel shows when the server answered with an error rather than a
   * page. It stands on the same shell as an invitation, because for most of these
   * the reader is not signed in and has nowhere else in the panel to be.
   *
   * The card's contents are `ErrorCard`, which the invitation page shows too - see
   * the note there. The server's own message is deliberately not among them; see
   * lib/panel-error.ts.
   */
  import type { PanelApi } from '../lib/api';
  import { panelUrl, type PanelBuild } from '../lib/base';
  import { describeFailure, type PanelFailure } from '../lib/panel-error';
  import ErrorCard from './ErrorCard.svelte';
  import NightPage from './NightPage.svelte';

  const {
    api,
    base,
    build,
    failure,
  }: { api: PanelApi; base: string; build: PanelBuild; failure: PanelFailure } = $props();

  const content = $derived(describeFailure(failure));
</script>

<NightPage title={content.title} documentTitle={content.title} {build}>
  <ErrorCard {content} panelHref={panelUrl(base, '/')} signInHref={api.signInUrl()} />
</NightPage>

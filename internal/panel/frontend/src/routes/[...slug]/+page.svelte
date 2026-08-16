<script lang="ts">
  import PanelApp from '$lib/App.svelte';
  import ErrorPage from '$lib/components/ErrorPage.svelte';
  import InvitationPage from '$lib/components/InvitationPage.svelte';
  import { createPanelApi } from '$lib/api';
  import { readBasePath, readPanelBuild } from '$lib/base';
  import { readPanelFailure } from '$lib/panel-error';
  import { createPanelRouter, parseInvitationToken } from '$lib/routes';

  const base = readBasePath(document);
  const api = createPanelApi(base, (input, init) => fetch(input, init));
  const build = readPanelBuild(document);
  // The service worker is disabled until Phase 8 migrates it to SvelteKit's
  // $service-worker module. The old sw.js fetches cache-manifest.json, which
  // SvelteKit does not emit, so registering it now would fail silently.
  // The server serves this same bundle when it is answering with an error, and
  // says so in the document. That is checked before the address is, because the
  // address is what failed: a 404 arrives at a path that looks like a panel route
  // and a failed sign-in arrives back at one, and neither should be booted into.
  const failure = readPanelFailure(document);
  const invitationToken = parseInvitationToken(base, window.location.pathname);
  // Built from the mount point rather than imported, because the route helpers
  // must not assume a baked base.
  const router = createPanelRouter(base, window);
</script>

{#if failure !== null}
  <ErrorPage {api} {base} {build} {failure} />
{:else if invitationToken === null}
  <PanelApp {api} {base} {build} {router} />
{:else}
  <InvitationPage {api} {base} token={invitationToken} {build} />
{/if}

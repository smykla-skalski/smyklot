<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { createPanelApi } from '#lib/api.js';
  import { readPanelBuild } from '#lib/base.js';
  import { basePath } from '#lib/paths.js';
  import InvitationPage from '#lib/components/InvitationPage.svelte';

  const api = createPanelApi(basePath, (input, init) => fetch(input, init));
  const build = readPanelBuild(document);
  const token = $derived(page.params.token ?? '');
</script>

<InvitationPage {api} base={basePath} {token} {build} />

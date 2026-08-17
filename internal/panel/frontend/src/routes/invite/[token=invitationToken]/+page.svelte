<script lang="ts">
  import { createPanelApi } from '#lib/api.js';
  import { readPanelBuild } from '#lib/base.js';
  import { basePath } from '#lib/paths.js';
  import InvitationPage from '#lib/components/InvitationPage.svelte';

  import type { PageProps } from './$types';

  const { params }: PageProps = $props();

  const api = createPanelApi(basePath, (input, init) => fetch(input, init));
  const build = readPanelBuild(document);
</script>

<InvitationPage {api} base={basePath} token={params.token} {build} />

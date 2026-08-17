<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { createPanelApi } from '$lib/api';
  import { readPanelBuild } from '$lib/base';
  import InvitationPage from '$lib/components/InvitationPage.svelte';

  const api = createPanelApi(resolve(''), (input, init) => fetch(input, init));
  const build = readPanelBuild(document);
  const token = $derived(page.params.token ?? '');
</script>

<InvitationPage {api} base={resolve('')} {token} {build} />

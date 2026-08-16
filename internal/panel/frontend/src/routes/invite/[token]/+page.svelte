<script lang="ts">
  import { page } from '$app/state';
  import { base } from '$app/paths';
  import { createPanelApi } from '$lib/api';
  import { readPanelBuild } from '$lib/base';
  import InvitationPage from '$lib/components/InvitationPage.svelte';

  const api = createPanelApi(base, (input, init) => fetch(input, init));
  const build = readPanelBuild(document);
  const token = $derived(page.params.token ?? '');
</script>

<InvitationPage {api} {base} {token} {build} />

<script lang="ts">
  import { page } from '$app/state';
  import InstallationView from '$lib/components/InstallationView.svelte';
</script>

<!-- A view on its own. Nothing follows it here, which is what makes an address
     that puts something after a view with no dialog resolve to no route. -->
<InstallationView view={page.params.view as string} />

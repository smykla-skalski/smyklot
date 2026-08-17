<script lang="ts">
  import InstallationView from '#lib/components/InstallationView.svelte';

  import type { PageProps } from './$types';

  const { params }: PageProps = $props();
</script>

<!-- A view on its own. Nothing follows it here, which is what makes an address
     that puts something after a view with no dialog resolve to no route. -->
<InstallationView view={params.view} />

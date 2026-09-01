<script lang="ts">
  import WorkspaceView from '#lib/components/WorkspaceView.svelte';

  import type { PageProps } from './$types';

  const { params }: PageProps = $props();
</script>

<!-- A view on its own. Nothing follows it here, which is what makes an address
     that puts something after a view with no dialog resolve to no route. -->
<WorkspaceView view={params.view} />

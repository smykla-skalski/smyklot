<script lang="ts">
  import WorkspaceView from '#lib/components/WorkspaceView.svelte';

  import type { PageProps } from './$types';

  const { params }: PageProps = $props();
</script>

{#key params.section}
  <WorkspaceView view="queue" />
{/key}

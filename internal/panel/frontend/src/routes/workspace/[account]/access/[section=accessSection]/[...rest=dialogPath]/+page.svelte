<script lang="ts">
  import WorkspaceView from '#lib/components/WorkspaceView.svelte';

  import type { PageProps } from './$types';

  const { params }: PageProps = $props();
</script>

<WorkspaceView view={params.section} />

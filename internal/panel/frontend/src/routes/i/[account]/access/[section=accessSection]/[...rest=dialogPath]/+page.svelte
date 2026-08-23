<script lang="ts">
  import InstallationView from '#lib/components/InstallationView.svelte';

  import type { PageProps } from './$types';

  const { params }: PageProps = $props();
</script>

<InstallationView view={params.section} />

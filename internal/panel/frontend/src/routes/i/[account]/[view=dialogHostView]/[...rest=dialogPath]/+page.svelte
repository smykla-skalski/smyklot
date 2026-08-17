<script lang="ts">
  import InstallationView from '#lib/components/InstallationView.svelte';

  import type { PageProps } from './$types';

  const { params }: PageProps = $props();
</script>

<!-- A view with a dialog standing on it. Only the views that host one are
     routed this way; `dialogHostView` is built from the list that says which. -->
<InstallationView view={params.view} />

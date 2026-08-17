<script lang="ts">
  import { page } from '$app/state';
  import InstallationView from '$lib/components/InstallationView.svelte';
</script>

<!-- A view with a dialog standing on it. Only the views that host one are
     routed this way; `dialogHostView` is built from the list that says which. -->
<InstallationView view={page.params.view as string} />

<script lang="ts">
  import InstallationView from '$lib/components/InstallationView.svelte';
</script>

<!-- History is routed by name because what follows it is a section rather than a
     dialog, and there are exactly two of those. A third answers 404. -->
<InstallationView view="history" />

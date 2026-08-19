<script lang="ts">
  import InstallationView from '#lib/components/InstallationView.svelte';
</script>

<!-- The files section, on the list or on one template. The path keeps its own
     separators rather than travelling percent-encoded: the Go server matches
     the decoded address, so a `%2F` in the tail is a path it cannot find. -->
<InstallationView view="sync" />

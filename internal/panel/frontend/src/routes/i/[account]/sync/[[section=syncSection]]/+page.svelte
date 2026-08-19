<script lang="ts">
  import InstallationView from '#lib/components/InstallationView.svelte';
</script>

<!-- One section of sync, named by the address. Nothing follows a section here:
     the two that can name one of their own - a ruleset, a file - are routed
     from their own directories, which is what keeps `/sync/files` from matching
     two routes at once. The bare address is the overview. -->
<InstallationView view="sync" />

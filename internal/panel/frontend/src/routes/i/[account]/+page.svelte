<script lang="ts">
  import InstallationView from '#lib/components/InstallationView.svelte';
</script>

<!-- The workspace opens here. The overview takes the bare address rather than a
     written one, the way the console's own overview takes `/root`: a page that
     is where you land does not also need a name to be told to ignore. -->
<InstallationView view="overview" />

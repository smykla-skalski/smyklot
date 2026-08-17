<script lang="ts">
  import InstallationView from '#lib/components/InstallationView.svelte';
</script>

<!-- One repository's own page. It renders the repositories view, which reads the
     repository out of the address and draws it in place of the list: the
     navigation still says Repositories, because that is where this is. -->
<InstallationView view="repositories" />

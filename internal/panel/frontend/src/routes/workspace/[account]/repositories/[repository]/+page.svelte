<script lang="ts">
  import WorkspaceView from '#lib/components/WorkspaceView.svelte';
</script>

<!-- One repository's own page. It renders the repositories view, which reads the
     repository out of the address and draws it in place of the list: the
     navigation still says Repositories, because that is where this is. -->
<WorkspaceView view="repositories" />

<script lang="ts">
  import WorkspaceView from '#lib/components/WorkspaceView.svelte';
</script>

<!-- History is routed by name because what follows it is a section rather than a
     dialog, and there are exactly two of those. A third answers 404. -->
<WorkspaceView view="history" />

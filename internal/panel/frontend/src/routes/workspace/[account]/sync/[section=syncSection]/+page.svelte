<script lang="ts">
  import WorkspaceView from '#lib/components/WorkspaceView.svelte';
</script>

<!-- A section of the sync view: a sidebar row with an address of its own. The
     section itself rides `page.params`, which the session reads back through
     `panelRouteAt` - the view component asks the session, not this page. -->
<WorkspaceView view="sync" />

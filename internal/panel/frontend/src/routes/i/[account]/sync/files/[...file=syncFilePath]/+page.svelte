<script lang="ts">
  import InstallationView from '#lib/components/InstallationView.svelte';
</script>

<!-- One template's own page, a level below the files list. The path rides
     `page.params` as the rest segment, which the session reads back through
     `panelRouteAt` - the view component asks the session, not this page. -->
<InstallationView view="sync" />

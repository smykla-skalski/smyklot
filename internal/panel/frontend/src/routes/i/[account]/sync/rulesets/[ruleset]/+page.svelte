<script lang="ts">
  import InstallationView from '#lib/components/InstallationView.svelte';
</script>

<!-- One ruleset's own page, a level below the rulesets list. The name rides
     `page.params`, which the session reads back through `panelRouteAt` - the
     view component asks the session, not this page. -->
<InstallationView view="sync" />

<script lang="ts">
  import InstallationView from '#lib/components/InstallationView.svelte';
</script>

<!-- The rulesets section, on the list or on one ruleset. A ruleset is keyed by
     name and nothing else, which is what the address can carry - GitHub mints
     the id, and an id in an address is a link nobody can read. -->
<InstallationView view="sync" />

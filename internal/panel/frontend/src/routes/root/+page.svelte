<script lang="ts">
  import { getPanelSession } from '#lib/session.svelte.js';
  import RootOverview from '#lib/components/RootOverview.svelte';

  const session = getPanelSession();
</script>

<div class="root-workspace">
  <RootOverview api={session.api} />
</div>

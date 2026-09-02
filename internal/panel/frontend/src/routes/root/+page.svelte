<script lang="ts">
  import { getPanelSession } from '#lib/session.svelte.js';
  import RootOverview from '#lib/components/RootOverview.svelte';

  const session = getPanelSession();
</script>

<section class="root-workspace" aria-labelledby="root-page-heading">
  <RootOverview api={session.api} />
</section>

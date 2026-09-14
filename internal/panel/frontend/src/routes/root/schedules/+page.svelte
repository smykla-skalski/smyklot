<script lang="ts">
  import SchedulesView from '#lib/components/SchedulesView.svelte';
  import { getPanelSession } from '#lib/session.svelte.js';

  const session = getPanelSession();
</script>

<div class="root-workspace">
  <SchedulesView api={session.api} />
</div>

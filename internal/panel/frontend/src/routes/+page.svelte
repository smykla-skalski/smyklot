<script lang="ts">
  import { goto } from '$app/navigation';
  import { getContext } from 'svelte';
  import type { PanelSession } from '$lib/session.svelte';

  const session = getContext<PanelSession>('panel-session');

  let redirected = false;
  $effect(() => {
    if (redirected || session.loading || session.viewer === null) return;
    if (session.viewer.system_role !== 'none') {
      redirected = true;
      goto(session.rootDashboardHref());
    } else if (session.targets.length > 0) {
      redirected = true;
      goto(session.targetHref(session.targets[0]));
    }
  });
</script>

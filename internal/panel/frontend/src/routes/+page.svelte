<script lang="ts">
  import { getPanelSession } from '$lib/session.svelte';

  const session = getPanelSession();

  let redirected = false;
  $effect(() => {
    if (redirected || session.loading || session.viewer === null) return;
    if (session.viewer.system_role !== 'none') {
      redirected = true;
      session.enterRoot();
    } else if (session.targets.length > 0) {
      redirected = true;
      void session.openTarget(session.targets[0]);
    }
  });
</script>

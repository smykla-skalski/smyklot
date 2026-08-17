<script lang="ts">
  import { getPanelSession } from '#lib/session.svelte.js';

  const session = getPanelSession();
  const { children } = $props();

  $effect(() => {
    if (session.viewer !== null && session.viewer.system_role === 'none' && !session.loading) {
      session.returnToPanel(true);
    }
  });
</script>

{@render children()}

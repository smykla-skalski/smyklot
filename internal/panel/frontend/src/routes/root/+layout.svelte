<script lang="ts">
  import { goto } from '$app/navigation';
  import { getContext } from 'svelte';
  import type { PanelSession } from '$lib/session.svelte';

  const session = getContext<PanelSession>('panel-session');
  const { children } = $props();

  $effect(() => {
    if (session.viewer !== null && session.viewer.system_role === 'none' && !session.loading) {
      goto(session.returnHref());
    }
  });
</script>

{@render children()}

<script lang="ts">
  import { getContext } from 'svelte';
  import type { PanelSession } from '$lib/session.svelte';
  import InboxView from '$lib/components/InboxView.svelte';

  const session = getContext<PanelSession>('panel-session');
</script>

<div id="inbox-panel">
  <InboxView
    fetchPage={session.api.fetchNotifications}
    markRead={session.api.markNotificationRead}
    onUnread={(count: number) => {
      session.notificationUnread = count;
    }}
  />
</div>

<style>
  #inbox-panel {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-height: 0;
  }
</style>

<script lang="ts">
  import { getPanelSession } from '$lib/session.svelte';
  import InboxView from '$lib/components/InboxView.svelte';

  const session = getPanelSession();
</script>

<div id="inbox-panel">
  <InboxView
    fetchPage={session.api.fetchNotifications}
    markRead={session.api.markNotificationRead}
    onUnread={(count: number) => session.updateNotificationUnread(count)}
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

<script lang="ts">
  import { getContext } from 'svelte';
  import type { PanelSession } from '$lib/session.svelte';
  import InboxView from '$lib/components/InboxView.svelte';

  const session = getContext<PanelSession>('panel-session');
</script>

<div id="inbox-panel">
  {#key session.notificationVersion}
    <InboxView
      fetchPage={session.api.fetchNotifications}
      markRead={session.api.markNotificationRead}
      refreshVersion={session.notificationVersion}
      onUnread={(count: number) => {
        session.notificationUnread = count;
      }}
    />
  {/key}
</div>

<style>
  #inbox-panel {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-height: 0;
  }
</style>

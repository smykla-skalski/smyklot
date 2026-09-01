<script lang="ts">
  import { panelAddress } from '#lib/addresses.js';
  import InboxView from '#lib/components/InboxView.svelte';
  import { getPanelSession } from '#lib/session.svelte.js';
  import type { SecurityNotification } from '#lib/types.js';

  const session = getPanelSession();

  const viewerName = $derived(
    session.viewer === null
      ? 'Personal'
      : session.viewer.account.display_name || session.viewer.account.login,
  );

  /**
   * The audit entry a receipt came from, when the reader can still reach it.
   *
   * A workspace they have since been removed from is one they cannot open, so the row
   * says what happened without offering a door that answers "not found".
   */
  function auditHref(notification: SecurityNotification): string | undefined {
    const login = notification.workspace.login;
    const known = session.targets.some((target) => target.account.login === login);

    return known ? panelAddress({ account: login, view: 'history', section: 'audit' }) : undefined;
  }
</script>

<div id="inbox-panel">
  <InboxView
    fetchPage={session.api.fetchNotifications}
    markRead={session.api.markNotificationRead}
    markAllRead={session.api.markAllNotificationsRead}
    {viewerName}
    {auditHref}
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

<script lang="ts">
  import { useQueryClient } from '@tanstack/svelte-query';
  import { invalidateRootInstallationSettings } from '#lib/query-client.js';
  import { getPanelSession } from '#lib/session.svelte.js';
  import { ROOT_SETTINGS_SCOPE } from '#lib/runtime-settings.js';
  import { getSettingsDraftRegistry } from '#lib/settings-drafts.svelte.js';
  import type {
    DeliveryFailure,
    InstallationSettingsBatchResponse,
    RootRuntimeSettings,
  } from '#lib/types.js';
  import HistoryPanel from '#lib/components/HistoryPanel.svelte';

  import type { PageProps } from './$types';

  const { params }: PageProps = $props();
  const session = getPanelSession();
  const queryClient = useQueryClient();
  const settingsDrafts = getSettingsDraftRegistry();
  // History opens on its first table when the address does not name one.
  const section = $derived(params.section ?? 'audit');

  function runtimeSettingsRestored(settings: RootRuntimeSettings): void {
    queryClient.setQueryData(['root-settings'], settings);
    void queryClient.invalidateQueries({ queryKey: ['root-overview'] });
  }

  function installationSettingsRestored(
    _settings: InstallationSettingsBatchResponse,
    targetId: string,
  ): void {
    void Promise.all([
      invalidateRootInstallationSettings(queryClient, targetId),
      queryClient.invalidateQueries({ queryKey: ['repository', targetId] }),
      queryClient.invalidateQueries({ queryKey: ['sync-override', targetId] }),
      queryClient.invalidateQueries({ queryKey: ['sync-plan', targetId] }),
      queryClient.invalidateQueries({ queryKey: ['audit', targetId] }),
      queryClient.invalidateQueries({ queryKey: ['audit', 'root'] }),
    ]);
  }

  function hasUnsavedInstallationSettings(targetId: string): boolean {
    return settingsDrafts.hasDirty({ type: 'installation', targetId });
  }

  /* The console can open the repository a failure was about, but only through the
     workspace that holds it - so a row whose workspace the catalog no longer names
     offers nothing rather than an address that resolves to somebody else's. */
  function repositoryPage(failure: DeliveryFailure): string | null {
    const account = failure.installation?.login;
    if (account === undefined) return null;

    return session.rootRepositoryHref(
      account,
      failure.repository_full_name.slice(failure.repository_full_name.lastIndexOf('/') + 1),
    );
  }
</script>

<section
  class="root-workspace"
  class:root-table-view={session.tableScrollView}
  aria-labelledby="root-page-heading"
>
  <HistoryPanel
    context="root"
    targetId="root"
    {section}
    fetchAudit={session.api.fetchRootAudit}
    fetchFailures={session.api.fetchRootFailures}
    fetchSettingsCheckpoint={session.api.fetchRootInstallationSettingsCheckpoint}
    restoreSettingsCheckpoint={session.api.restoreRootInstallationSettingsCheckpoint}
    fetchRootSettingsCheckpoint={session.api.fetchRootRuntimeSettingsCheckpoint}
    fetchRootSettingsBaseline={session.api.fetchRootRuntimeSettingsBaseline}
    restoreRootSettingsCheckpoint={session.api.restoreRootRuntimeSettingsCheckpoint}
    hasUnsavedRootSettingsDrafts={settingsDrafts.hasDirty(ROOT_SETTINGS_SCOPE)}
    hasUnsavedSettingsDraftsForTarget={hasUnsavedInstallationSettings}
    readOnly={false}
    onSettingsRestored={installationSettingsRestored}
    onRootSettingsRestored={runtimeSettingsRestored}
    repositoryHref={repositoryPage}
  />
</section>

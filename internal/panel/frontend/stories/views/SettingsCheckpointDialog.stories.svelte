<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SettingsCheckpointDialog from '#lib/components/SettingsCheckpointDialog.svelte';
  import type { SettingsCheckpoint } from '#lib/types.js';
  import { ACCOUNT } from '../support/fixtures.js';

  const checkpoint: SettingsCheckpoint = {
    id: 'checkpoint-42',
    action: 'installation.settings.saved',
    actor: ACCOUNT,
    created_at: '2026-08-23T08:00:00Z',
    affected_kinds: ['target', 'repository', 'sync_config'],
    items: [
      {
        kind: 'target',
        document_version: 1,
        before: {
          available: true,
          state: {
            document: {
              repository_default_enabled: false,
              pending_ci_mode_default: 'checks',
              config_patch: {},
            },
            digest: 'target-before',
            revision: 3,
          },
          differs: false,
          restorable: true,
        },
        after: {
          available: true,
          state: {
            document: {
              repository_default_enabled: true,
              pending_ci_mode_default: 'checks',
              config_patch: { quiet_success: true },
            },
            digest: 'target-after',
            revision: 4,
          },
          differs: true,
          restorable: true,
        },
        current: {
          document: {
            repository_default_enabled: false,
            pending_ci_mode_default: 'checks',
            config_patch: {},
          },
          digest: 'target-current',
          revision: 8,
        },
        changed: true,
      },
      {
        kind: 'repository',
        repository_id: 'repo-1',
        repository_full_name: 'smykla-skalski/smyklot',
        document_version: 1,
        before: { available: true, state: null, differs: true, restorable: true },
        after: {
          available: true,
          state: {
            document: {
              enabled_override: true,
              ignore_repository_file: false,
              config_patch: {},
            },
            digest: 'repository-after',
            revision: 2,
          },
          differs: false,
          restorable: true,
        },
        current: {
          document: {
            enabled_override: true,
            ignore_repository_file: false,
            config_patch: {},
          },
          digest: 'repository-after',
          revision: 2,
        },
        changed: true,
      },
      {
        kind: 'sync_config',
        sync_kind: 'labels',
        document_version: 0,
        before: { available: true, state: null, differs: true, restorable: true },
        after: {
          available: true,
          state: {
            document: {
              enabled: true,
              document: JSON.stringify({
                labels: [{ name: 'ci/green' }],
                allow_removal: true,
                excludes: [],
              }),
            },
            digest: 'labels-after',
            revision: 3,
          },
          differs: true,
          restorable: false,
          incompatibility: {
            code: 'document_version',
            reason: 'This snapshot uses an unsupported document version',
          },
        },
        current: {
          document: { enabled: false, document: '{}' },
          digest: 'labels-current',
          revision: 5,
        },
        changed: true,
      },
    ],
  };

  const base = {
    open: true,
    identity: 'target-1',
    checkpointId: checkpoint.id,
    readOnly: false,
    hasUnsavedDrafts: false,
    fetchCheckpoint: async () => checkpoint,
    restoreCheckpoint: async () => {},
    onClose: fn(),
  };

  const { Story } = defineMeta({
    title: 'Views/SettingsCheckpointDialog',
    component: SettingsCheckpointDialog,
    args: base,
  });
</script>

<Story name="Restore available" />
<Story name="Unsaved settings" args={{ hasUnsavedDrafts: true }} />
<Story name="Inspection only" args={{ readOnly: true, restoreCheckpoint: undefined }} />

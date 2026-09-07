<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';
  import ConfigurationFileSync from '#lib/components/ConfigurationFileSync.svelte';
  import { mockConfigFileStatus, type ConfigFileStatusVariant } from '../../dev/config-file-status';

  function connection(variant: ConfigFileStatusVariant) {
    return {
      data: mockConfigFileStatus(variant, 'smykla-skalski/smyklot'),
      isPending: false,
      isFetching: false,
      error: null,
      refetch: fn(),
    };
  }
  const { Story } = defineMeta({
    title: 'Views/ConfigurationFileSync',
    component: ConfigurationFileSync,
    args: {
      scope: 'repository',
      repository: 'smykla-skalski/smyklot',
      enabled: true,
      savedEnabled: true,
      now: Date.parse('2026-09-07T12:05:00Z'),
      onChange: fn(),
      connection: connection('ready'),
    },
  });
</script>

<Story name="In sync" />
<Story name="Off" args={{ enabled: false, savedEnabled: false, connection: connection('off') }} />
<Story
  name="Unsaved start"
  args={{ savedEnabled: false, dirty: true, connection: connection('off') }}
/>
<Story name="Unsaved stop" args={{ enabled: false, dirty: true }} />
<Story name="Sync pending" args={{ connection: connection('syncing') }} />
<Story name="Pending" args={{ connection: connection('pending') }} />
<Story name="Stale check" args={{ connection: connection('stale') }} />
<Story name="Proposed" args={{ connection: connection('proposed') }} />
<Story name="Conflicting changes" args={{ connection: connection('conflict') }} />
<Story name="Removed file" args={{ connection: connection('removed') }} />
<Story name="Invalid TOML" args={{ connection: connection('invalid') }} />
<Story name="Invalid schema" args={{ connection: connection('schema') }} />
<Story name="Invalid scope" args={{ connection: connection('scope') }} />
<Story
  name="File settings off"
  args={{ fileIgnored: true, savedFileIgnored: true, connection: connection('fileOff') }}
/>
<Story name="Unavailable" args={{ connection: connection('unavailable') }} />
<Story name="Outstanding proposal" args={{ connection: connection('outstanding') }} />
<Story name="Workspace" args={{ scope: 'workspace', repository: 'smykla-skalski/.github' }} />
<Story
  name="Missing workspace repository"
  args={{
    scope: 'workspace',
    repository: 'smykla-skalski/.github',
    connection: connection('missingAccess'),
  }}
/>
<Story name="Read only" args={{ readOnly: true }} />
<Story
  name="Load error"
  args={{
    connection: { ...connection('ready'), error: new Error('GitHub access is unavailable') },
  }}
/>

<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import RepositoryControl from '#lib/components/RepositoryControl.svelte';
  import type { RepositoryDetail } from '#lib/types.js';
  import {
    REPOSITORY_FILE_SEARCH_PATHS,
    REPOSITORY_FILE_VARIANTS,
  } from '../../dev/repository-files.js';
  import { REPOSITORY, REPOSITORY_DETAIL } from '../support/fixtures.js';

  const NOW = Date.parse('2026-09-06T12:00:00Z');

  function fileVariant(name: string): RepositoryDetail {
    const variant = REPOSITORY_FILE_VARIANTS[name];
    return {
      ...REPOSITORY_DETAIL,
      config_file_observation: {
        status: variant.status,
        search_paths: REPOSITORY_FILE_SEARCH_PATHS,
        ...(variant.status === 'unknown'
          ? {}
          : { observed_at: new Date(NOW - 5 * 60_000).toISOString() }),
      },
      config_file_path: variant.path,
      config_file_patch: variant.patch,
      config_file_error: variant.error,
      config_file_superseded: variant.superseded,
      ignore_repository_file: variant.bypass ?? false,
      config_migration: variant.migration ?? 'none',
      config_migration_pr: variant.migrationPR,
    };
  }

  const { Story } = defineMeta({
    title: 'Views/RepositoryControl',
    component: RepositoryControl,
    args: {
      repository: REPOSITORY,
      detail: fileVariant('feature-flags'),
      now: NOW,
      onEnablement: fn(),
      onUseFile: fn(),
      onResetMigration: fn(),
    },
  });
</script>

<Story name="Valid file" />
<Story name="Valid empty file" args={{ detail: fileVariant('auth-service') }} />
<Story name="Valid file ignored" args={{ detail: fileVariant('api-gateway') }} />
<Story name="Missing file" args={{ detail: fileVariant('billing-worker') }} />
<Story name="Missing file ignored" args={{ detail: fileVariant('event-consumer') }} />
<Story name="Not checked" args={{ detail: fileVariant('data-pipeline') }} />
<Story name="Invalid file" args={{ detail: fileVariant('cli-tools') }} />
<Story name="Invalid file ignored" args={{ detail: fileVariant('customer-portal') }} />
<Story name="Other detected files" args={{ detail: fileVariant('edge-proxy') }} />
<Story name="Migration proposal open" args={{ detail: fileVariant('deployment-config') }} />
<Story name="Migration proposal closed" args={{ detail: fileVariant('design-system') }} />
<Story name="Migration proposal blocked" args={{ detail: fileVariant('docs-site') }} />
<Story name="Retrying proposal" args={{ detail: fileVariant('docs-site'), busy: true }} />
<Story name="Smyklot on" args={{ enablement: 'enabled' }} />
<Story name="Smyklot off" args={{ enablement: 'disabled' }} />
<Story
  name="Unsaved choices"
  args={{ enablement: 'disabled', dirtyEnabled: true, dirtyUseFile: true }}
/>
<Story name="Read only" args={{ detail: fileVariant('docs-site'), readOnly: true }} />
<Story
  name="File inspector"
  args={{ detail: fileVariant('edge-proxy') }}
  play={async ({ canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole('button', { name: 'Inspect file' }));
  }}
/>
<Story
  name="Unchecked file inspector"
  args={{ detail: fileVariant('data-pipeline') }}
  play={async ({ canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole('button', { name: 'Inspect file' }));
  }}
/>

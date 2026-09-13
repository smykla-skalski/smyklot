<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SyncCheckSummary from '#lib/components/SyncCheckSummary.svelte';
  import { queueSeeds } from '../../dev/fixtures';
  import { NOW } from '../support/fixtures.js';
  const item = {
    ...queueSeeds((offset) => new Date(NOW + offset).toISOString())[0]!,
    kind: 'sync_scan' as const,
    state: 'succeeded' as const,
    details: {
      outcome: {
        completed_at: new Date(NOW).toISOString(),
        disposition: 'checked',
        summary: 'Compared repositories. One category could not be checked.',
        counts: { matched: 8, different: 1, failed: 1 },
        cached: 2,
        missing_permissions: ['rulesets'],
      },
    },
  };
  const { Story } = defineMeta({
    title: 'Views/SyncCheckSummary',
    component: SyncCheckSummary,
    args: { item },
  });
</script>

<Story name="Mixed outcome" />
<Story name="Legacy check" args={{ item: { ...item, details: {} } }} />
<Story
  name="Running"
  args={{ item: { ...item, state: 'running', summary: undefined, details: {} } }}
/>

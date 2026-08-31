<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import SyncKindFacts from '#lib/components/SyncKindFacts.svelte';

  import { NOW, SYNC_STATUS } from '../support/fixtures.js';

  const { Story } = defineMeta({
    title: 'Primitives/SyncKindFacts',
    component: SyncKindFacts,
    argTypes: {
      kind: { control: 'select', options: ['labels', 'settings', 'rulesets', 'files'] },
      enabled: { control: 'boolean' },
    },
    args: {
      kind: 'labels' as const,
      enabled: true,
      status: SYNC_STATUS,
      updatedBy: 'Bart Smykla',
      updatedAt: new Date(NOW - 2 * 3_600_000).toISOString(),
      nowMs: NOW,
    },
  });
</script>

<!--
  The line under a kind's switch. Three roles - how far the kind reaches, what
  pausing costs, who last changed it - each at its own ink weight, because a
  bare toggle tells a reader nothing about the open plan it does not cancel.
-->
<Story name="Syncing" />

<Story name="Paused" args={{ enabled: false }} />

<!-- Never configured: there is no history to show, and the line says the rest. -->
<Story name="Never changed" args={{ updatedBy: '', updatedAt: '' }} />

<!-- The fleet has not been read yet, so the reach is not claimed. -->
<Story name="Fleet unknown" args={{ status: null }} />

<Story name="Repository options" args={{ kind: 'settings' as const }} />

<Story name="Shared files, paused" args={{ kind: 'files' as const, enabled: false }} />

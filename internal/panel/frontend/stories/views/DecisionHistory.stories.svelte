<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import DecisionHistory from '#lib/components/DecisionHistory.svelte';
  import Seeded from '../support/Seeded.svelte';
  import { ACCOUNT, NOW } from '../support/fixtures.js';
  import type { AccessDecision } from '#lib/types.js';

  const at = (offsetMs: number) => new Date(NOW + offsetMs).toISOString();

  const DECISIONS: AccessDecision[] = [
    {
      id: 'd-1',
      actor: ACCOUNT,
      action: 'access.granted',
      summary: 'Granted Editor, joining the platform team',
      created_at: at(-3 * 24 * 60 * 60_000),
    },
    {
      id: 'd-2',
      actor: ACCOUNT,
      action: 'access.revoked',
      summary: 'Revoked Editor, left the team',
      created_at: at(-60 * 60_000),
    },
  ];

  const KEY = ['access-decisions', 'ada'] as const;

  const { Story } = defineMeta({
    title: 'Views/DecisionHistory',
    component: DecisionHistory,
    args: {
      open: true,
      label: '@ada',
      scopeLabel: 'Smykla Skalski',
      status: 'active',
      queryKey: KEY,
      fetchDecisions: async () => DECISIONS,
      onClose: fn(),
    },
  });
</script>

<!--
  Why somebody has the access they have, in the order it was decided. It is a dialog,
  so it portals to `.app-shell` and takes whichever palette is active.
-->
<Story name="With a history">
  {#snippet template(args)}
    <Seeded seed={[[KEY, DECISIONS]]}>
      <DecisionHistory {...args} />
    </Seeded>
  {/snippet}
</Story>

<!-- Access that has never been changed: there is nothing to show, and it says so. -->
<Story name="Nothing decided">
  {#snippet template(args)}
    <Seeded seed={[[KEY, []]]}>
      <DecisionHistory {...args} fetchDecisions={async () => []} />
    </Seeded>
  {/snippet}
</Story>

<!-- Still reading: the dialog opens on a placeholder rather than on an empty state. -->
<Story name="Loading">
  {#snippet template(args)}
    <Seeded>
      <DecisionHistory {...args} fetchDecisions={() => new Promise(() => {})} />
    </Seeded>
  {/snippet}
</Story>

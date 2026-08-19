<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Button from '#lib/components/Button.svelte';
  import SyncBoard, { type BoardRepository } from '#lib/components/SyncBoard.svelte';

  const { Story } = defineMeta({
    title: 'Views/SyncBoard',
    component: SyncBoard,
  });

  const settled = (name: string): BoardRepository => ({ name, state: 'settled' });

  /** A believable fleet: mostly settled, which is what makes four stand out. */
  const FLEET: BoardRepository[] = [
    settled('.github'),
    { name: 'af', state: 'change', changes: 6 },
    { name: 'afi', state: 'change', changes: 5 },
    { name: 'harness', state: 'change', changes: 3 },
    {
      name: 'smyklot-legacy',
      state: 'refused',
      reason: '.github/workflows/ci.yaml needs the workflows permission',
    },
    { name: 'dotfiles', state: 'off' },
    ...[
      'sai',
      'klaudiush',
      'smyklot',
      'orca',
      'docs',
      'infra',
      'charts',
      'actions',
      'tooling',
      'bench',
      'probe',
      'relay',
      'skald',
      'forge',
      'mirror',
      'quill',
      'spore',
      'weft',
      'lattice',
    ].map(settled),
  ];

  const SETTLED_FLEET: BoardRepository[] = FLEET.map(({ name }) => settled(name));
</script>

<!--
  The fleet as a shape. One raised tile per repository in a sunken well, the
  numeral only where changes wait, a dashed socket where sync is off here.

  The legend is a key AND a filter - press "Would change" and every other tile
  dims, which is how four are found among twenty-five without a search box.
  Dimming rather than removing keeps the shape still under the hand.
-->
<Story name="An installation with a plan waiting">
  {#snippet template()}
    <SyncBoard
      repositories={FLEET}
      label="Repositories in this installation"
      footLine="14 changes across 3 repositories, including 1 removal"
      footWhen="Worked out 12 minutes ago · expires in 6 hours · nothing happens until you apply it"
    >
      <Button tone="brand">Review the plan</Button>
    </SyncBoard>
  {/snippet}
</Story>

<!-- Nothing to do: no numerals, no foot, and the board says so by being quiet. -->
<Story name="Everything in step">
  {#snippet template()}
    <SyncBoard repositories={SETTLED_FLEET} label="Repositories in this installation" />
  {/snippet}
</Story>

<!-- A young installation, where the board is a handful of tiles rather than a wall. -->
<Story name="A small installation">
  {#snippet template()}
    <SyncBoard
      repositories={[
        settled('smyklot'),
        { name: 'panel', state: 'change', changes: 2 },
        { name: 'sandbox', state: 'off' },
      ]}
      label="Repositories in this installation"
      footLine="2 changes in 1 repository"
      footWhen="Worked out a minute ago"
    >
      <Button tone="brand">Review the plan</Button>
    </SyncBoard>
  {/snippet}
</Story>

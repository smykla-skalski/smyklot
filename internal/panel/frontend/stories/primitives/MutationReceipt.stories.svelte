<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Button from '#lib/components/Button.svelte';
  import MutationReceipt from '#lib/components/MutationReceipt.svelte';
  import { receipts } from '#lib/receipts.svelte.js';

  const { Story } = defineMeta({
    title: 'Primitives/MutationReceipt',
    component: MutationReceipt,
  });
</script>

<script lang="ts">
  /* The receipt draws itself at the foot of the WINDOW, so a story that only mounted it
     would photograph an empty frame with something in the corner of the screen. Each of
     these says one, and gives the reader the way to say it again. */
  function removal(): void {
    receipts.say(
      'Removed release-please - the next plan deletes it in all 25 syncing repositories',
      {
        undo: () => receipts.say('release-please is back'),
      },
    );
  }

  function saved(): void {
    receipts.say('Saved - CI re-checks runs every 30 seconds, around the clock from the next run');
  }

  function held(): void {
    receipts.say('Pausing background work - nothing new starts until an operator resumes it', {
      sticky: true,
      undo: () => receipts.say('Background work is running again'),
    });
  }
</script>

<Story name="A change with a way back">
  {#snippet template()}
    <div class="stage">
      <p>A change that can be taken back says so, and the way back is on the receipt:</p>
      <Button onclick={removal}>Remove a label</Button>
      <MutationReceipt />
    </div>
  {/snippet}
</Story>

<Story name="A change that simply happened">
  {#snippet template()}
    <div class="stage">
      <p>A change with nothing to undo is still reported, in the words of the thing it changed:</p>
      <Button onclick={saved}>Save a schedule</Button>
      <MutationReceipt />
    </div>
  {/snippet}
</Story>

<Story name="A receipt that keeps the floor">
  {#snippet template()}
    <div class="stage">
      <p>A receipt somebody may still act on stays until it is answered, and holds the line:</p>
      <Button onclick={held}>Pause background work</Button>
      <MutationReceipt />
    </div>
  {/snippet}
</Story>

<style>
  .stage {
    display: grid;
    gap: var(--space-3);
    justify-items: start;
    max-inline-size: 34rem;
  }
</style>

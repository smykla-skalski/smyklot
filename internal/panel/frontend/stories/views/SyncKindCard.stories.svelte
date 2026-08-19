<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import SyncKindCard from '#lib/components/SyncKindCard.svelte';
  import type { SyncState } from '#lib/components/StateMark.svelte';

  const { Story } = defineMeta({
    title: 'Views/SyncKindCard',
    component: SyncKindCard,
  });

  /** Twenty-five slots, in the board's order. `at` says which ones are not settled. */
  const strip = (marks: Record<number, SyncState>): SyncState[] =>
    Array.from({ length: 25 }, (_, index) => marks[index] ?? 'settled');

  const LABELS = strip({ 5: 'off', 10: 'off' });
  const SETTINGS = strip({ 1: 'change', 2: 'change', 10: 'off' });
  const RULESETS = strip({ 2: 'change', 10: 'off' });
  const FILES = strip({ 1: 'change', 3: 'change', 4: 'refused', 5: 'off', 10: 'off' });
</script>

<script lang="ts">
  let labels = $state(true);
  let files = $state(true);
  let rulesets = $state(false);
</script>

<!--
  The four kinds as they stand on the overview. The strip under each summary is
  the board again, same repositories in the same order - so the repository that
  is refused in Files sits in the same column in all four cards, and a glance
  down answers "which kinds is it failing".
-->
<Story name="The four kinds">
  {#snippet template()}
    <div class="kind-grid">
      <SyncKindCard
        name="Labels"
        href="#labels"
        summary="12 labels · removal off"
        states={LABELS}
        when="bart, 2 hours ago"
        enabled={labels}
        onToggle={(next) => {
          labels = next;
        }}
      />
      <SyncKindCard
        name="Settings"
        href="#settings"
        summary="9 of 17 managed, the rest follow each repository"
        states={SETTINGS}
        when="bart, yesterday"
      />
      <SyncKindCard
        name="Rulesets"
        href="#rulesets"
        summary="2 rulesets · 1 evaluating"
        states={RULESETS}
        when="bart, 3 days ago"
        enabled={rulesets}
        onToggle={(next) => {
          rulesets = next;
        }}
      />
      <SyncKindCard
        name="Files"
        href="#files"
        summary="5 templates · 1 retired path · changes arrive as pull requests"
        states={FILES}
        when="bart, 20 minutes ago"
        enabled={files}
        onToggle={(next) => {
          files = next;
        }}
      />
    </div>
  {/snippet}
</Story>

<!-- One card, switched off: it keeps its place and goes quiet rather than leaving. -->
<Story name="Switched off">
  {#snippet template()}
    <div class="one">
      <SyncKindCard
        name="Rulesets"
        href="#rulesets"
        summary="2 rulesets · nothing is planned while this is off"
        states={RULESETS}
        when="bart, 3 days ago"
        enabled={false}
      />
    </div>
  {/snippet}
</Story>

<style>
  .kind-grid {
    display: grid;
    gap: var(--space-4);
    grid-template-columns: repeat(4, 1fr);
  }

  .one {
    max-width: 18rem;
  }

  @media (max-width: 64rem) {
    .kind-grid {
      grid-template-columns: repeat(2, 1fr);
    }
  }
</style>

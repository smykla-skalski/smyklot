<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import RemovalPolicy from '#lib/components/RemovalPolicy.svelte';

  const { Story } = defineMeta({
    title: 'Views/RemovalPolicy',
    component: RemovalPolicy,
  });
</script>

<script lang="ts">
  // Live, because the exemptions are what makes the switch above them safe to
  // turn on, and the card only reads right when both move.
  let removal = $state(false);
  let excludes = $state<string[]>(['hand-made-*']);
  let rulesetRemoval = $state(true);
  let rulesetExcludes = $state<string[]>([]);
</script>

<!--
  Off, which is the state every kind starts in: a repository keeps whatever it
  has that this list does not name.
-->
<Story name="Removal off">
  {#snippet template()}
    <RemovalPolicy
      noun="labels"
      {removal}
      {excludes}
      onRemovalChange={(next) => (removal = next)}
      onExcludesChange={(next) => (excludes = next)}
    />
  {/snippet}
</Story>

<!--
  On, with nothing exempt - the one arrangement that deletes. The exemptions sit
  under the switch rather than on another card because somebody who can turn
  this on from a page has to be able to protect something from that page too.
-->
<Story name="Removal on, nothing exempt">
  {#snippet template()}
    <RemovalPolicy
      noun="rulesets"
      removal={rulesetRemoval}
      excludes={rulesetExcludes}
      onRemovalChange={(next) => (rulesetRemoval = next)}
      onExcludesChange={(next) => (rulesetExcludes = next)}
    />
  {/snippet}
</Story>

<!-- A save in flight: the controls stand still rather than queueing writes. -->
<Story name="While saving">
  {#snippet template()}
    <RemovalPolicy
      noun="labels"
      removal
      excludes={['hand-made-*', 'release/*']}
      disabled
      onRemovalChange={() => {}}
      onExcludesChange={() => {}}
    />
  {/snippet}
</Story>

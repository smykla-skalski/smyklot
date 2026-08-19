<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import ApplyBar from '#lib/components/ApplyBar.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/ApplyBar',
    component: ApplyBar,
    args: { changes: 14, repositories: 3, removals: 1, asPullRequests: true },
  });
</script>

<!--
  The last thing read before something reaches GitHub, so it names the blast
  radius twice: in the sentence, and in the button. "Apply" alone is a button
  that cannot be checked against what it is about to do.
-->
<Story name="Playground">
  {#snippet template(args)}
    <ApplyBar {...args} />
  {/snippet}
</Story>

<!-- Nothing is being taken away, so nothing is said in the danger ink. -->
<Story name="Additions only">
  {#snippet template()}
    <ApplyBar changes={6} repositories={2} />
  {/snippet}
</Story>

<!-- One repository, one change: the sentence and the button both say so. -->
<Story name="A single repository">
  {#snippet template()}
    <ApplyBar changes={1} repositories={1} removals={1} />
  {/snippet}
</Story>

<!-- In flight: the act is named in the present tense and the control goes quiet. -->
<Story name="Applying">
  {#snippet template()}
    <ApplyBar changes={14} repositories={3} removals={1} asPullRequests applying />
  {/snippet}
</Story>

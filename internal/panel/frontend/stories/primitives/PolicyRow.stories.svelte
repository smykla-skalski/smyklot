<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import PolicyRow from '#lib/components/PolicyRow.svelte';
  import Switch from '#lib/components/Switch.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/PolicyRow',
    component: PolicyRow,
  });
</script>

<script lang="ts">
  let on = $state(true);
  let method = $state('squash');
</script>

<!--
  One thing this installation decides. The value is said as a word AND as the
  control, because a column of switches is decoded and a column of words is
  read. The clear is quiet until the row is under the hand.
-->
<Story name="A boolean this installation decides">
  {#snippet template()}
    <PolicyRow
      name="Delete the branch on merge"
      why="The head branch goes as soon as its pull request lands"
      value={on ? 'On' : 'Off'}
      onStopManaging={() => {}}
    >
      {#snippet control()}
        <Switch
          checked={on}
          ariaLabel="Delete the branch on merge"
          onChange={(next) => {
            on = next;
          }}
        />
      {/snippet}
    </PolicyRow>
  {/snippet}
</Story>

<!-- A value with more than two states says itself in the same place. -->
<Story name="A choice of more than two">
  {#snippet template()}
    <PolicyRow
      name="Merge method"
      why="How a pull request lands when Smyklot merges it"
      onStopManaging={() => {}}
    >
      {#snippet control()}
        <select bind:value={method} aria-label="Merge method">
          <option value="squash">Squash</option>
          <option value="merge">Merge commit</option>
          <option value="rebase">Rebase</option>
        </select>
      {/snippet}
    </PolicyRow>
  {/snippet}
</Story>

<!-- Where the decision cannot be given up, nothing offers to give it up. -->
<Story name="Without a way to stop managing">
  {#snippet template()}
    <PolicyRow name="Secret scanning" value="On">
      {#snippet control()}
        <Switch checked ariaLabel="Secret scanning" onChange={() => {}} />
      {/snippet}
    </PolicyRow>
  {/snippet}
</Story>

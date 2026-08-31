<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Chip from '#lib/components/Chip.svelte';
  import Pill, { type PillTone } from '#lib/components/Pill.svelte';

  const TONES: PillTone[] = ['bare', 'role', 'success', 'warning', 'danger', 'info', 'neutral'];

  const { Story } = defineMeta({
    title: 'Primitives/Pill',
    component: Pill,
    argTypes: {
      tone: { control: 'select', options: TONES },
    },
    args: { tone: 'bare' },
  });
</script>

<!--
  `children` comes out of the destructure and is never spread back in - the same reason
  `Chip`'s story does it: Storybook puts every required prop into `args`, so spreading
  would set the snippet twice and the markup between the tags would lose.
-->
<Story name="Playground">
  {#snippet template({ children, ...args })}
    <Pill {...args}>Editor</Pill>
  {/snippet}
</Story>

<Story name="All tones">
  {#snippet template()}
    <div class="row">
      {#each TONES as tone (tone)}
        <Pill {tone}>{tone}</Pill>
      {/each}
    </div>
  {/snippet}
</Story>

<!-- The comparison the app can never show on one page, and the whole reason there are
     two families: a pill is a STANDING and is fully round, a chip is a VALUE and is a
     rounded rect with a keyline you can press. -->
<Story name="Against a chip">
  {#snippet template()}
    <div class="row">
      <Pill tone="role">Owner</Pill>
      <Pill>Editor</Pill>
      <Pill tone="warning">Suspended</Pill>
      <Chip small>sync:managed</Chip>
      <Chip small tone="accent">3 overrides</Chip>
    </div>
  {/snippet}
</Story>

<!-- How a person's row wears them: the role first, the state only when there is one. -->
<Story name="On a row">
  {#snippet template()}
    <div class="card">
      <ul class="object-list">
        <li>
          <div class="object-row">
            <span class="object-main">
              <span class="object-name-row">
                <span class="object-name">Bart Smykla</span>
                <Pill tone="role">Owner</Pill>
              </span>
              <span class="object-sum">@bart · last opened 6 minutes ago</span>
            </span>
            <span class="object-side"></span>
          </div>
        </li>
        <li>
          <div class="object-row">
            <span class="object-main">
              <span class="object-name-row">
                <span class="object-name">Margaret Hamilton</span>
                <Pill>Viewer</Pill>
                <Pill tone="warning">Suspended</Pill>
              </span>
              <span class="object-sum">
                @margaret · suspended 2 days ago - sign-in is refused until an administrator lifts
                it
              </span>
            </span>
            <span class="object-side"></span>
          </div>
        </li>
      </ul>
    </div>
  {/snippet}
</Story>

<style>
  .row {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
</style>

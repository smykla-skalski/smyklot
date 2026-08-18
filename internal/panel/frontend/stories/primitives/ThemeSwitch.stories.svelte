<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import ThemeSwitch from '#lib/components/ThemeSwitch.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/ThemeSwitch',
    component: ThemeSwitch,
    argTypes: {
      theme: { control: 'inline-radio', options: ['system', 'light', 'dark'] },
      surface: { control: 'inline-radio', options: ['panel', 'sidebar', 'night'] },
      system: { control: 'boolean' },
    },
    args: { name: 'story-theme', theme: 'system', surface: 'panel', system: true, onSelect: fn() },
  });
</script>

<Story name="Playground" />

<!-- Without the system option the control offers only the two explicit answers. -->
<Story name="Light and dark only" args={{ theme: 'dark', system: false }} />

<!--
  The switch is drawn on three grounds, and each has its own track and thumb values -
  the sidebar and the night sky are not the panel's surface.
-->
<Story name="On each surface">
  {#snippet template()}
    <div class="stack">
      <ThemeSwitch name="s-panel" theme="system" surface="panel" onSelect={() => {}} />
      <ThemeSwitch name="s-sidebar" theme="system" surface="sidebar" onSelect={() => {}} />
      <ThemeSwitch name="s-night" theme="system" surface="night" onSelect={() => {}} />
    </div>
  {/snippet}
</Story>

<style>
  .stack {
    display: grid;
    gap: var(--space-3);
    justify-items: start;
  }
</style>

<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import SectionTabs from '#lib/components/SectionTabs.svelte';

  const SYNC_TABS = [
    { id: 'overview', label: 'Overview', href: '#overview' },
    { id: 'labels', label: 'Labels', href: '#labels' },
    { id: 'settings', label: 'Settings', href: '#settings' },
    { id: 'rulesets', label: 'Rulesets', href: '#rulesets' },
    { id: 'files', label: 'Files', href: '#files' },
    { id: 'plan', label: 'Plan', href: '#plan', count: '14', signal: true },
  ];

  const { Story } = defineMeta({
    title: 'Primitives/SectionTabs',
    component: SectionTabs,
    args: { items: SYNC_TABS, active: 'overview', label: 'Sync sections' },
  });
</script>

<script lang="ts">
  let active = $state('overview');
</script>

<!--
  Navigation, not filtering: every tab is an address rendered as a real link
  with aria-current. The open tab says it twice - weight and the ink bar - and
  the bar is deliberately not the brand colour: "you are here" is not "you can
  act". Click around the Live story to see the bar slide and the labels hold
  their width (each reserves its bold copy).
-->
<Story name="Live">
  {#snippet template()}
    <SectionTabs
      items={SYNC_TABS}
      {active}
      label="Sync sections"
      onNavigate={(id) => (active = id)}
    />
  {/snippet}
</Story>

<Story name="Plan waiting">
  {#snippet template()}
    <SectionTabs items={SYNC_TABS} active="plan" label="Sync sections" onNavigate={() => {}} />
  {/snippet}
</Story>

<Story name="Quiet counts">
  {#snippet template()}
    <SectionTabs
      items={[
        { id: 'open', label: 'Open', href: '#open', count: '12' },
        { id: 'closed', label: 'Closed', href: '#closed', count: '241' },
        { id: 'all', label: 'All', href: '#all' },
      ]}
      active="open"
      label="Demo sections"
      onNavigate={() => {}}
    />
  {/snippet}
</Story>

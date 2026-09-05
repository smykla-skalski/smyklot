<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import DisclosureSection from '#lib/components/DisclosureSection.svelte';
  import Card from '#lib/components/Card.svelte';
  const { Story } = defineMeta({
    title: 'Primitives/DisclosureSection',
    component: DisclosureSection,
  });
</script>

<Story name="Optional settings">
  {#snippet template()}
    <DisclosureSection
      title="Formatting preferences"
      description="Advanced · inherits workspace defaults"
    >
      <Card
        ><div class="card-head"><h2 class="card-title">JSON</h2></div>
        <span class="object-sum">Expanded cards keep their own frame and the standard spacing.</span
        ></Card
      >
      <Card
        ><div class="card-head"><h2 class="card-title">Markdown</h2></div>
        <span class="object-sum">The disclosure adds no enclosing panel.</span></Card
      >
    </DisclosureSection>
  {/snippet}
</Story>

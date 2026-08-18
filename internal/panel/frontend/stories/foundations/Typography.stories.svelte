<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import TokenTable from '../support/TokenTable.svelte';

  const SCALE = [
    { name: 'font-size-page-title', note: 'The page heading' },
    { name: 'font-size-title', note: 'A section' },
    { name: 'font-size-dialog-title', note: 'A dialog' },
    { name: 'font-size-card-title', note: 'A card' },
    { name: 'font-size-body', note: 'Running text and a row headline' },
    { name: 'font-size-meta', note: 'Descriptions, secondary cells, form errors' },
    { name: 'font-size-compact', note: 'Controls' },
    { name: 'font-size-control', note: 'A control inside a row' },
    { name: 'font-size-micro', note: 'Uppercase eyebrows and pills' },
    { name: 'font-size-nano', note: 'The smallest thing the panel draws' },
  ];

  const FAMILIES = [
    { name: 'sans', note: 'Plus Jakarta Sans - everything the interface says' },
    { name: 'mono', note: 'JetBrains Mono - a login, an id, a value read character by character' },
  ];

  const SIZES = [
    'font-size-page-title',
    'font-size-title',
    'font-size-body',
    'font-size-meta',
    'font-size-compact',
    'font-size-micro',
  ];

  const { Story } = defineMeta({ title: 'Foundations/Typography' });
</script>

<Story name="Scale">
  {#snippet template()}
    <TokenTable tokens={SCALE} swatch={false} />
  {/snippet}
</Story>

<Story name="Families">
  {#snippet template()}
    <TokenTable tokens={FAMILIES} swatch={false} />
  {/snippet}
</Story>

<!--
  Every line here is trimmed with `text-box: trim-both cap alphabetic`, so each box
  starts at the cap and ends on the baseline. That is what lets the panel centre text
  against anything beside it without a nudge - the box IS the band.
-->
<Story name="Specimens">
  {#snippet template()}
    <div class="specimens">
      {#each SIZES as size (size)}
        <p style:font-size={`var(--${size})`}>
          <span class="cap-trim">Repositories waiting on checks</span>
        </p>
      {/each}
      <p class="mono"><span class="cap-trim">@smykla-skalski · #3001</span></p>
    </div>
  {/snippet}
</Story>

<style>
  .specimens {
    display: grid;
    gap: var(--space-4);
  }
  .specimens p {
    margin: 0;
  }
</style>

<script lang="ts">
  import type { PanelApi } from '../api';
  import { panelUrl, type PanelBuild } from '../base';
  import { describeFailure, type PanelFailure } from '../panel-error';
  import Card from './Card.svelte';
  import ErrorCard from './ErrorCard.svelte';
  import NightPage from './NightPage.svelte';
  import PageHeader from './PageHeader.svelte';

  const {
    api,
    base,
    build,
    failure,
    insidePanel = false,
    destinations = [],
  }: {
    api: PanelApi;
    base: string;
    build: PanelBuild;
    failure: PanelFailure;
    /** Keep the signed-in shell's landmarks and footer when a child route fails. */
    insidePanel?: boolean;
    /** Pages that do exist, for a reader who is already inside one console. */
    destinations?: ReadonlyArray<{ label: string; href: string }>;
  } = $props();

  const content = $derived(describeFailure(failure));
</script>

<!--
@component
What the panel shows when the server answered with an error rather than a
page. It stands on the same shell as an invitation, because for most of these
the reader is not signed in and has nowhere else in the panel to be.

The card's contents are `ErrorCard`, which the invitation page shows too - see
the note there. The server's own message is deliberately not among them; see
lib/panel-error.ts.
-->

<svelte:head>
  {#if insidePanel}
    <title>{content.title} | SMYKLOT</title>
  {/if}
</svelte:head>

{#if insidePanel}
  <!-- A READER WHO IS ALREADY INSIDE IS ON A PAGE, not on an error screen. The shell is
       still around them and they still know where they are, so this says what happened in
       the page's own voice and names somewhere that does exist - rather than a centred
       plate with a five-rem status number, which is what a reader arriving cold gets.

       The destinations are links in a row and not the design's `fact-bit`s: that dot is a
       verdict token everywhere else in the sheet, and a green one beside "Repositories"
       on a not-found page reads as a report on the repositories. -->
  <div class="view-frame">
    <PageHeader id="panel-error-heading" title={content.title} />
    <Card>
      <div class="state-panel">
        <span><strong>{content.lead}.</strong> {content.note}</span>
      </div>
      {#if destinations.length > 0}
        <p class="error-elsewhere">
          {#each destinations as destination (destination.href)}
            <a href={destination.href}>{destination.label}</a>
          {/each}
        </p>
      {/if}
    </Card>
  </div>
{:else}
  <NightPage title={content.title} documentTitle={content.title} {build}>
    <ErrorCard {content} panelHref={panelUrl(base, '/')} signInHref={api.signInUrl()} />
  </NightPage>
{/if}

<style>
  /* One line of destinations under the sentence, on the card's own band. */
  .error-elsewhere {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    font-size: var(--font-size-meta);
    gap: var(--space-2) var(--space-5);
    margin: var(--space-4) 0 0;
  }

  /* A link is a control on a phone too. */
  .error-elsewhere a {
    align-items: center;
    display: inline-flex;
    min-block-size: var(--tier-quiet);
  }
</style>

<script lang="ts">
  /**
   * What the panel shows when the server answered with an error rather than a
   * page. It stands on the same shell as an invitation, because for most of these
   * the reader is not signed in and has nowhere else in the panel to be.
   *
   * The card's contents are `ErrorCard`, which the invitation page shows too - see
   * the note there. The server's own message is deliberately not among them; see
   * lib/panel-error.ts.
   */
  import type { PanelApi } from '../api';
  import { panelUrl, type PanelBuild } from '../base';
  import { describeFailure, type PanelFailure } from '../panel-error';
  import ErrorCard from './ErrorCard.svelte';
  import NightPage from './NightPage.svelte';
  import Plate from './Plate.svelte';

  const {
    api,
    base,
    build,
    failure,
    insidePanel = false,
  }: {
    api: PanelApi;
    base: string;
    build: PanelBuild;
    failure: PanelFailure;
    /** Keep the signed-in shell's landmarks and footer when a child route fails. */
    insidePanel?: boolean;
  } = $props();

  const content = $derived(describeFailure(failure));
</script>

<svelte:head>
  {#if insidePanel}
    <title>{content.title} | SMYKLOT</title>
  {/if}
</svelte:head>

{#if insidePanel}
  <div class="panel-error">
    <Plate label={content.title} tone="alarm">
      <ErrorCard {content} panelHref={panelUrl(base, '/')} signInHref={api.signInUrl()} />
    </Plate>
  </div>
{:else}
  <NightPage title={content.title} documentTitle={content.title} {build}>
    <ErrorCard {content} panelHref={panelUrl(base, '/')} signInHref={api.signInUrl()} />
  </NightPage>
{/if}

<style>
  /* The surrounding workspace already owns the main landmark, full-height shell,
     and footer. Centre only the recovery surface inside the space it gives us. */
  .panel-error {
    align-content: center;
    display: grid;
    flex: 1;
    margin-inline: auto;
    max-width: 42rem;
    padding-block: var(--space-8);
    width: 100%;
  }
</style>

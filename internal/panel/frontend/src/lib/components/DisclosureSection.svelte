<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';

  const {
    title,
    description,
    children,
  }: {
    title: string;
    description?: string;
    children: Snippet;
  } = $props();
</script>

<!--
@component
An optional section whose contents are complete cards. Its summary wears the
shared card surface; its expanded body keeps the cards as siblings, without a
second frame around them. Use a plain Card for controls that should always be
visible, or the existing card.fold anatomy when the disclosure itself holds rows.
The native disclosure owns keyboard activation and its expanded state.
-->
<details class="disclosure-section">
  <summary class="card">
    <span class="card-head">
      <span class="disclosure-chevron"><Icon name="chevron-right" size="xs" /></span>
      <span class="card-title">{title}</span>
      {#if description}<span class="card-note band-trim">{description}</span>{/if}
    </span>
  </summary>
  <div class="card-stack disclosure-body">{@render children()}</div>
</details>

<style>
  .disclosure-section {
    min-inline-size: 0;
  }
  summary {
    cursor: pointer;
    list-style: none;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      box-shadow var(--duration-press) var(--ease-standard);
  }
  summary::-webkit-details-marker {
    display: none;
  }
  summary:hover {
    background: var(--row-hover);
  }
  summary:active {
    background: var(--row-pressed);
    box-shadow: var(--pressed-inset);
  }
  summary > .card-head {
    margin: 0;
  }
  .disclosure-chevron {
    display: grid;
    place-items: center;
    color: var(--text-muted);
  }
  details[open] .disclosure-chevron {
    rotate: 90deg;
  }
  .disclosure-body {
    grid-template-columns: minmax(0, 1fr);
    min-inline-size: 0;
    margin-block-start: var(--rhythm-card-gap);
  }
  details:not([open]) > .disclosure-body {
    display: none;
  }
</style>

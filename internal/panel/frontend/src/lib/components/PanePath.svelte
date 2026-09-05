<script module lang="ts">
  /** One pressable ancestor on the way back up. */
  export interface PanePathSegment {
    label: string;
    href: string;
    onSelect?: () => void;
  }
</script>

<script lang="ts">
  import Icon from './Icon.svelte';
  const {
    segments,
  }: {
    /**
     * The page's ancestors, nearest console root first. Root pages carry
     * none: the shell already says the console, and a path pointing at the
     * page it sits on is a self-link.
     */
    segments: readonly PanePathSegment[];
  } = $props();

  function plainClick(event: MouseEvent, segment: PanePathSegment): void {
    if (segment.onSelect === undefined) return;
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
      return;
    }
    event.preventDefault();
    segment.onSelect();
  }
</script>

<!--
@component
The way back up: hierarchy, not history. A page below its console's root opens with
its ancestors, each one pressable, and the last of them is Up - the browser keeps
Back, and the two answer different questions.

A root page carries none, which is why an empty `segments` renders nothing at all
rather than an empty strip. The shell already says which console you are in, and a
path pointing at the page it sits on is a self-link.
-->

{#if segments.length > 0}
  <nav class="pane-path" aria-label="Where this page sits">
    {#each segments as segment, index (segment.label + segment.href)}
      {#if index > 0}
        <span class="path-sep" aria-hidden="true">/</span>
      {/if}
      <a class="crumb" href={segment.href} onclick={(event) => plainClick(event, segment)}>
        {#if segments.length === 1}<Icon name="chevron-left" size="sm" />{/if}
        <span class="t">{segment.label}</span>
      </a>
    {/each}
  </nav>
{/if}

<style>
  .pane-path {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .crumb {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--text-secondary);
    font-size: var(--font-size-micro);
    font-weight: 600;
    letter-spacing: 0.07em;
    text-transform: uppercase;
    padding: var(--space-2);
    margin: calc(var(--space-2) * -1);
    border-radius: var(--r-ctl);
    text-decoration: none;
    transition:
      background var(--duration-fast) var(--ease-standard),
      color var(--duration-fast) var(--ease-standard);
  }
  .crumb:hover {
    background: var(--row-hover);
    color: var(--text-primary);
  }
  .crumb:active {
    background: var(--row-pressed);
  }
  .crumb .t {
    text-box: trim-both cap alphabetic;
  }
  .path-sep {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
  }
</style>

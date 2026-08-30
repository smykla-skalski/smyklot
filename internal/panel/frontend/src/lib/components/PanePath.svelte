<script module lang="ts">
  /** One pressable ancestor on the way back up. */
  export interface PanePathSegment {
    label: string;
    href: string;
    onSelect?: () => void;
  }
</script>

<script lang="ts">
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
        <span class="t">{segment.label}</span>
      </a>
    {/each}
  </nav>
{/if}

<style>
  /* Every page below its console's root opens with its ancestors - each one
     pressable. The way up is an eyebrow, not a sentence: small caps, like
     every other wayfinding label. */
  .pane-path {
    align-items: center;
    display: flex;
    gap: 0.4rem;
    /* A 24px hit area on each segment without moving a pixel of layout. */
    margin-block: -6px calc(var(--space-3) - 6px);
  }

  .crumb {
    align-items: center;
    color: var(--text-secondary);
    display: inline-flex;
    gap: 0.5rem;
    font-size: var(--font-size-meta);
    padding-block: 6px;
    text-decoration: none;
    transition:
      color var(--duration-fast) var(--ease-standard),
      translate var(--duration-press) var(--ease-standard);
  }

  .crumb:hover {
    color: var(--text-primary);
  }

  .crumb:active {
    translate: 0 1px;
  }

  /* Floored like every other trimmed line: the micro cap is 8.2, and the
     floor keeps the path's declared height whole (6 + 9 + 6 = 21). */
  .crumb .t {
    font-size: var(--font-size-micro);
    font-weight: 600;
    letter-spacing: 0.07em;
    min-block-size: 9px;
    text-box: trim-both cap alphabetic;
    text-transform: uppercase;
  }

  .path-sep {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
    font-weight: 600;
    min-block-size: 9px;
    text-box: trim-both cap alphabetic;
  }
</style>

<script module lang="ts">
  /** One entry: the id of the section it names, and the word it is named by. */
  export interface TocEntry {
    id: string;
    label: string;
  }
</script>

<script lang="ts">
  import { onMount, tick } from 'svelte';
  const {
    title = 'On this page',
    entries,
  }: {
    /** The column's own label. Not a link, and not one of the entries. */
    title?: string;
    entries: readonly TocEntry[];
  } = $props();

  let column = $state<HTMLElement | null>(null);
  let here = $state<string | null>(null);

  /**
   * AN INDEX FOR A PAGE THAT FITS ON THE SCREEN IS A LIST OF THINGS THE READER CAN
   * ALREADY SEE, so it is offered only where the page scrolls. This is the one thing
   * here JavaScript decides, and it is not the kind the layout laws forbid: nothing
   * measures to place anything. CSS cannot answer "does this document scroll" - there
   * is no query for the relationship between a document's height and the viewport's -
   * so an observer asks, and the answer is a single class on the frame. Where it sits
   * and how it sticks is still the sheet's.
   */
  function tell(): void {
    const frame = column?.parentElement;
    if (frame === null || frame === undefined) return;
    const scrolls = document.documentElement.scrollHeight > window.innerHeight + 1;
    // WRITE ONLY ON A CHANGE. Showing the index changes what the observer below is
    // watching, so an unconditional toggle answers its own notification and the browser
    // reports the loop as an error.
    if (frame.classList.contains('page-scrolls') === scrolls) return;
    frame.classList.toggle('page-scrolls', scrolls);
  }

  /** The reader's place: the last section whose top has crossed the reading line. */
  function mark(): void {
    const visible = entries.filter((entry) => {
      const section = document.getElementById(entry.id);
      return section !== null && section.getClientRects().length > 0;
    });
    if (window.scrollY <= 1) {
      here = visible[0]?.id ?? null;
      return;
    }
    if (window.scrollY + window.innerHeight >= document.documentElement.scrollHeight - 1) {
      here = visible.at(-1)?.id ?? null;
      return;
    }
    let found: string | null = null;
    for (const entry of visible) {
      const section = document.getElementById(entry.id);
      if (section === null || section.getClientRects().length === 0) continue;
      const inset = Number.parseFloat(getComputedStyle(section).scrollMarginBlockStart) || 0;
      if (section.getBoundingClientRect().top <= inset + 1) {
        found = entry.id;
      }
    }
    here = found ?? entries[0]?.id ?? null;
  }

  function reveal(id: string): HTMLElement | null {
    const section = document.getElementById(id);
    if (!section) return null;
    let parent: HTMLElement | null = section;
    while (parent) {
      if (parent instanceof HTMLDetailsElement) parent.open = true;
      parent = parent.parentElement;
    }
    return section;
  }

  let anchor: { section: HTMLElement; y: number } | null = null;

  async function revealHash(): Promise<void> {
    anchor = null;
    const entry = entries.find((entry) => `#${entry.id}` === window.location.hash);
    if (entry) {
      const section = reveal(entry.id);
      await tick();
      section?.scrollIntoView();
      if (section) anchor = { section, y: window.scrollY };
    }
    sync();
  }

  onMount(() => void revealHash());

  /** Both answers depend on where the page's content currently is. */
  function sync(): void {
    if (anchor && Math.abs(window.scrollY - anchor.y) > 1) anchor = null;
    tell();
    mark();
  }

  function resize(): void {
    if (anchor && Math.abs(window.scrollY - anchor.y) <= 1 && anchor.section.isConnected) {
      anchor.section.scrollIntoView();
      anchor.y = window.scrollY;
    }
    sync();
  }

  $effect(() => {
    const frame = column?.parentElement;
    if (frame === null || frame === undefined) return;

    sync();
    /* `document.body` rather than `documentElement`: the root element's box is the
       viewport's and reports no change when the content grows or shrinks, so an observer
       on it answers once and never again. The body is what the content makes tall, and
       `frame` is what a filter or a fold changes underneath it. The window handlers
       below cover the same question at every scroll and resize, which is what keeps the
       answer right where an observer is throttled. */
    const observer = new ResizeObserver(resize);
    observer.observe(document.body);
    observer.observe(frame);

    return () => {
      observer.disconnect();
      frame.classList.remove('page-scrolls');
    };
  });
</script>

<!--
@component
The index beside a long page: where its sections are, and which one the reader is in.

It is drawn only where there is room for it outside the reading column and only where the
page actually scrolls - both decided by the sheet, from a class this sets. The entries
are plain fragment links, so the browser's own scrolling, history and focus handling are
what move the page. Selecting a section reveals any containing native disclosures.
At the document boundaries, the first or last visible section owns the marker;
elsewhere the last section crossing the top edge, including its scroll margin, owns it. The active link exposes
aria-current="location" for assistive technology.
-->

<svelte:window onscroll={sync} onresize={sync} onhashchange={revealHash} />

<nav class="page-toc" aria-label={title} bind:this={column}>
  <p class="toc-title">{title}</p>
  {#each entries as entry (entry.id)}
    <a
      href="#{entry.id}"
      onclick={(event) => {
        if (
          event.button === 0 &&
          !event.metaKey &&
          !event.ctrlKey &&
          !event.shiftKey &&
          !event.altKey
        )
          reveal(entry.id);
      }}
      aria-current={here === entry.id ? 'location' : undefined}
      class:is-here={here === entry.id}>{entry.label}</a
    >
  {/each}
</nav>

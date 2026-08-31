<script module lang="ts">
  /** One entry: the id of the section it names, and the word it is named by. */
  export interface TocEntry {
    id: string;
    label: string;
  }
</script>

<script lang="ts">
  const {
    title = 'On this page',
    entries,
  }: {
    /** The column's own label. Not a link, and not one of the entries. */
    title?: string;
    entries: readonly TocEntry[];
  } = $props();

  /** A third of the way down: a section scrolled to by its own link lands under the
   *  shell's air, and a line at zero would credit the section above it. */
  const READING_LINE = 0.33;

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
    let found: string | null = null;
    for (const entry of entries) {
      const section = document.getElementById(entry.id);
      if (section === null || section.getClientRects().length === 0) continue;
      if (section.getBoundingClientRect().top <= window.innerHeight * READING_LINE) {
        found = entry.id;
      }
    }
    here = found ?? entries[0]?.id ?? null;
  }

  /** Both answers depend on where the page's content currently is. */
  function sync(): void {
    tell();
    mark();
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
    const observer = new ResizeObserver(sync);
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
what move the page.
-->

<svelte:window onscroll={sync} onresize={sync} />

<nav class="page-toc" aria-label={title} bind:this={column}>
  <p class="toc-title">{title}</p>
  {#each entries as entry (entry.id)}
    <a href="#{entry.id}" class:is-here={here === entry.id}>{entry.label}</a>
  {/each}
</nav>

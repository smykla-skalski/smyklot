<script lang="ts">
  import type { Snippet } from 'svelte';

  /**
   * Who a row is about: a mark, a name, and the handle under it.
   *
   * Five tables wrote this by hand under five class names - `.identity`,
   * `.user-identity`, `.option-copy`, `.who-*`, `.workspace-identity` - and the
   * copies disagreed about the one thing that is hard to see and easy to lose.
   *
   * **The stack is trimmed, always.** A name over a handle is two elements, so the
   * container has no text of its own and `text-box` on it does nothing - it is a
   * flex container, and its lines are its children's. `.band-trim-stack` gives back
   * the leading above the first line's capitals and the room under the last line's
   * baseline, which is what makes the *words* sit on the mark's centre rather than
   * the box that holds them. Measured at 1.80px in the sidebar's workspace switcher
   * and 0.76px in the account card.
   *
   * Two of the five call sites had it and one did not. That one measured 0.000px
   * off centre anyway, because at its size the leading and the descender room
   * happened to cancel - correct by luck rather than by construction, and only for
   * that pairing of fonts at that size. Here it is by construction.
   */
  const {
    mark,
    name,
    handle,
    extra,
  }: {
    /** The avatar or monogram the row is led by. */
    mark: Snippet;
    /** Usually the display name, sometimes a link to it. */
    name: Snippet;
    /** The `@login`, and whatever else belongs on the second line. */
    handle: Snippet;
    /**
     * Anything that is not part of the stack - a hint for a screen reader, say.
     *
     * Outside the stack on purpose: `.band-trim-stack` trims the last child, and a
     * visually-hidden span is a last child with no line to give back.
     */
    extra?: Snippet;
  } = $props();
</script>

<!--
@component
An account said in one line: the avatar, the name and the handle, at the seven places
that report who something belongs to.

## The sidebar's account row is NOT one of these, and that was decided by looking

`IdentityBar`'s `.who` draws the same three things - avatar, name, handle - and is
the obvious eighth call site. It stays as it is, because the two are only the same
shape and not the same thing:

- Its ink is `--sidebar-text` and `--sidebar-text-muted`, a palette the rail
  re-declares. This row is written against the page's.
- Its stack gap is 0.1rem, measured against the card it sits in. This one takes
  `--space-2`, measured against a table row.
- Its two lines are a step smaller, because the rail is 240px and a name has to
  fit beside a 32px avatar with a chevron after it.

Folding them together would change how the sidebar looks, which is a design
decision wearing a refactor's clothes. It is worth asking whether the sidebar
should adopt this row's typography; it is worth asking where it can be looked
at, not in a diff that claims to move nothing.
-->

<span class="identity-row">
  {@render mark()}
  {#if extra !== undefined}
    {@render extra()}
  {/if}
  <span class="band-trim-stack">
    {@render name()}
    {@render handle()}
  </span>
</span>

<style>
  .identity-row {
    align-items: center;
    display: flex;
    gap: var(--space-2);
    min-width: 0;
  }

  /* Rendered here, so it carries this component's scope and needs no `:global`. */
  .band-trim-stack {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  /* The two lines come from the caller's snippets, so they wear the caller's scope
     class and a bare selector cannot reach them - which is what `:global` inside a
     scoped left-hand side is for. It is the component's own typography either way:
     the name is the row's headline, the handle is quieter under it, and both
     truncate rather than wrapping, because a row is one line tall.

     Two tables declared exactly this and a third wrote its own. Anything a caller
     genuinely needs to differ - the workspaces table's monospace handle - it
     still says for itself, on a class of its own. */
  /* `clip` with a margin, not `hidden`.
     ------------------------------------
     `.band-trim-stack` ends the last line's box on its baseline, which is what puts
     the pair on the mark's centre - but the trim moves the box and not the glyphs,
     so the `y` in a login and the tail of an `@` still paint below it. `hidden` cut
     them off along the bottom of every row, and Chrome is the only engine that
     implements the trim, so it was the only one showing it.

     Room outside the box rather than an open block axis: this is a table row, and
     ink that escaped would land in the row beneath. 0.4em is what the queue's
     pull-request names already ask for; the deepest descender here is 0.18em.

     The workspaces table had found this and the other two had not, so two of the
     three were clipping their own descenders. Here it is the default, which is the
     point of the component. */
  .identity-row > :global(.band-trim-stack) > :global(*) {
    overflow: clip;
    overflow-clip-margin: 0.4em;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .identity-row :global(strong) {
    font-size: var(--font-size-body);
    line-height: var(--leading-body);
  }

  .identity-row :global(.mono) {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    line-height: var(--leading-compact);
  }
</style>

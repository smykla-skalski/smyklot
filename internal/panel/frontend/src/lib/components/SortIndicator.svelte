<script lang="ts">
  import Icon from './Icon.svelte';

  /**
   * The arrow in a sortable column heading, after the word rather than before
   * it.
   *
   * One chevron, pointing up, turned over by `app.css` for descending and turned
   * again under a pointer to offer the direction a press would give.
   *
   * It was the `sort` glyph - an up chevron over a down one - rotated the same
   * way. That symbol is rotationally symmetric, so the rotation drew exactly
   * what it started with: the column said it could be sorted and never which way
   * it was. A one-directional glyph is the whole fix; the turning was always
   * right.
   *
   * The pair was tried again deliberately - both directions, always drawn, the
   * sorted one lit - which is what AG Grid and Ant Design draw and what the
   * WAI-ARIA sortable table example shows. Leading the word it read as clutter
   * at this type size, and Bart's call is the single arrow, trailing.
   *
   * Nothing is announced from here. The heading cell carries `aria-sort`, which
   * is what a screen reader reads, and saying it twice is a repetition rather
   * than a help.
   */
</script>

<span class="sort-indicator" aria-hidden="true"><Icon name="chevron-up" size={14} /></span>

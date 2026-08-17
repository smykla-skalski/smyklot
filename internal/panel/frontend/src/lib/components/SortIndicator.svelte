<script lang="ts">
  import Icon from './Icon.svelte';

  /**
   * The arrow in a sortable column heading.
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
   * Nothing is announced from here. The heading cell carries `aria-sort`, which
   * is what a screen reader reads, and saying it twice is a repetition rather
   * than a help.
   */
</script>

<span class="sort-indicator" aria-hidden="true"><Icon name="chevron-up" size={14} /></span>

<script lang="ts">
  import Icon from './Icon.svelte';

  const {
    title,
    description,
    actionLabel,
    onAction,
  }: {
    title: string;
    description: string;
    actionLabel?: string;
    onAction?: () => void;
  } = $props();
</script>

<!-- The shape is `.table-notice` in app.css, shared with the failure that stands
     in the same place. All this decides is the glyph and the words. -->
<div class="table-notice">
  <span class="table-notice-mark" aria-hidden="true"><Icon name="search" size={22} /></span>
  <strong>{title}</strong>
  <span>{description}</span>
  {#if actionLabel !== undefined && onAction !== undefined}
    <!-- The label is wrapped so `app.css` can trim it: a button centres its label
         BOX, and the box carries the leading above the capitals and the room under
         the baseline, which are never equal. Bare, this word sat 0.47px above the
         middle of its own surface. -->
    <button class="btn" type="button" onclick={onAction}>
      <span class="button-label">{actionLabel}</span>
    </button>
  {/if}
</div>

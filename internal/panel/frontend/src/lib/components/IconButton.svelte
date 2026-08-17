<script lang="ts">
  import Icon, { type IconName } from './Icon.svelte';

  const {
    icon,
    label,
    onclick,
    disabled = false,
    busy = false,
  }: {
    icon: IconName;
    /** Says what the button does. It is the accessible name and the tooltip both. */
    label: string;
    onclick: (event: MouseEvent) => void;
    disabled?: boolean;
    /** Turns the glyph while the work this button started is still in flight. */
    busy?: boolean;
  } = $props();
</script>

<!-- The shape and the states are `.icon-button` in app.css, shared with the menu
     trigger beside it and the filter triggers in the heading above. All this adds
     is the glyph and what happens when it is pressed. -->
<button
  type="button"
  class="icon-button"
  class:busy
  aria-busy={busy}
  aria-label={label}
  title={label}
  disabled={disabled || busy}
  {onclick}
>
  <Icon name={icon} size={14} strokeWidth={2} />
</button>

<style>
  /* Reduced motion is already honoured: app.css caps every animation at 0.01ms
     under the query, so this settles at its start angle rather than spinning. */
  .busy :global(svg) {
    animation: icon-button-spin 900ms linear infinite;
  }

  @keyframes icon-button-spin {
    to {
      rotate: 360deg;
    }
  }
</style>

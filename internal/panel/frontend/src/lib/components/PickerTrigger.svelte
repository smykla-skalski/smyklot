<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLButtonAttributes } from 'svelte/elements';

  import Button from './Button.svelte';
  import Icon from './Icon.svelte';

  const { children, ...rest }: { children: Snippet } & HTMLButtonAttributes = $props();
</script>

<!--
@component
A popover-backed choice uses the shared button's size and interaction states.
Forward the popover trigger attributes so focus, keyboard input and expanded state
remain owned by the popover. The trailing chevron keeps the shared control anatomy.
-->
<Button {...rest} class="value-select">
  {@render children()}
  {#snippet trailing()}<Icon name="chevron-down" size="sm" />{/snippet}
</Button>

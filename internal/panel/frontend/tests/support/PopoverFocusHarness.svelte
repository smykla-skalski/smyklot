<script lang="ts">
  import Button from '../../src/lib/components/Button.svelte';
  import Popover from '../../src/lib/components/Popover.svelte';

  const { onCloseAutoFocus }: { onCloseAutoFocus?: (event: Event) => void } = $props();
  let open = $state(false);
</script>

<input aria-label="Focus destination" />
<Popover bind:open role="dialog" label="Focus test" {onCloseAutoFocus}>
  {#snippet trigger(attributes)}<Button {...attributes}>Open choices</Button>{/snippet}
  <Button onclick={() => (open = false)}>Cancel</Button>
</Popover>

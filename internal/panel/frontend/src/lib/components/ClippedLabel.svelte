<script lang="ts">
  /**
   * A one-line label that knows when it has been cut. While the text fits,
   * this is a plain span; the moment the ellipsis appears, hovering the
   * label answers with the whole text in the app tooltip - and only then,
   * because a tip that repeats what is already fully visible is noise.
   *
   * The wrapper decides how the label clips (overflow, nowrap, ellipsis);
   * this component only measures the cut and carries the tip.
   */
  import type { Attachment } from 'svelte/attachments';

  import AppTooltip from './AppTooltip.svelte';

  const {
    text,
    class: cls = '',
  }: {
    text: string;
    class?: string;
  } = $props();

  let clipped = $state(false);

  /* Re-created whenever the text changes, so a swapped label re-measures;
     the observer covers the other way the cut changes - the box resizing. */
  // eslint-disable-next-line @typescript-eslint/no-unused-vars -- the argument exists to key the attachment to the text
  function measured(_: string): Attachment {
    return (element) => {
      const check = (): void => {
        clipped = element.scrollWidth > element.clientWidth + 1;
      };
      check();
      const watcher = new ResizeObserver(check);
      watcher.observe(element);
      return () => watcher.disconnect();
    };
  }
</script>

<AppTooltip {text} disabled={!clipped}>
  {#snippet children(attributes)}
    <span {...attributes} class={cls} {@attach measured(text)}>{text}</span>
  {/snippet}
</AppTooltip>

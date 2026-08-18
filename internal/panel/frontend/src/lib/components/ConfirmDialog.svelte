<script lang="ts">
  import type { Snippet } from 'svelte';

  import Button, { type ButtonTone } from './Button.svelte';
  import Modal from './Modal.svelte';

  /**
   * A dialog that asks before doing something, and the two buttons that end it.
   *
   * The bodies genuinely differ - one takes a reason for the audit record, one shows
   * a consequence, one shows nothing - so the body stays the caller's. What repeated
   * was the footer: a ghost Cancel that the dialog opens focused on, and a confirm
   * whose tone says whether the thing being done takes something away or gives it,
   * disabled while the request is in flight and reading differently while it is.
   *
   * Four of those were the same eleven lines under four names.
   *
   * Which control opens focused is Bits UI's to decide - the panel's own
   * `data-modal-focus` attribute turned out to be read by nothing at all and was
   * removed from all nine places that wrote it.
   */
  const {
    id,
    open,
    title,
    description,
    returnFocus = null,
    onClose,
    onConfirm,
    confirmLabel = 'Confirm',
    busyLabel = 'Saving…',
    confirmTone = 'default',
    busy = false,
    confirmDisabled = false,
    cancelLabel = 'Cancel',
    children,
  }: {
    id: string;
    open: boolean;
    title: string;
    description?: string;
    returnFocus?: HTMLElement | null;
    onClose: () => void;
    onConfirm: () => void;
    confirmLabel?: string;
    /** What the confirm reads while the request is in flight. */
    busyLabel?: string;
    /** `stop` for taking something away, `signal` for giving it. */
    confirmTone?: ButtonTone;
    busy?: boolean;
    /** For a form that is not yet answerable - an empty required field. */
    confirmDisabled?: boolean;
    cancelLabel?: string;
    children: Snippet;
  } = $props();
</script>

<Modal {id} {open} {title} {description} {returnFocus} {onClose}>
  {@render children()}

  {#snippet footer()}
    <Button tone="ghost" onclick={onClose}>{cancelLabel}</Button>
    <Button tone={confirmTone} disabled={busy || confirmDisabled} onclick={onConfirm}>
      {busy ? busyLabel : confirmLabel}
    </Button>
  {/snippet}
</Modal>

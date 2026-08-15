<script lang="ts">
  import { tick, type Snippet } from 'svelte';
  import { initialModalFocus, modalElementIds } from '../lib/modal';
  import Icon from './Icon.svelte';

  const {
    id,
    open,
    title,
    description,
    closeLabel = 'Close dialog',
    variant = 'dialog',
    returnFocus = null,
    onClose,
    children,
    footer,
    headerExtra,
  }: {
    id: string;
    open: boolean;
    title: string;
    description?: string;
    closeLabel?: string;
    variant?: 'dialog' | 'inspector' | 'wide';
    returnFocus?: HTMLElement | null;
    onClose: () => void;
    children: Snippet;
    footer?: Snippet;
    headerExtra?: Snippet;
  } = $props();

  let dialog = $state<HTMLDialogElement | null>(null);
  const elementIds = $derived(modalElementIds(id));

  /**
   * A dialog is moved out to the shell it belongs to, and taken away again by hand.
   *
   * `showModal()` promotes an element to the top layer, but it does not exempt it from an ancestor
   * that is not rendering: a dialog written inside a closed `<details>` measures 0x0 and paints
   * nothing. The inbox is written inside the account menu, so opening it had to leave that menu
   * hanging open behind it. The shell rather than `document.body`, because the design tokens are
   * declared on `.app-shell` - the Root console re-skins them there - and a dialog reparented to
   * the body would inherit the panel's palette inside the Root console.
   *
   * The teardown is not optional. Svelte removes a component's nodes from where it put them, so a
   * node moved somewhere else is not removed at all: without this, dismissing a modal that is
   * conditionally rendered left the dialog in the document, still open and still in the top layer,
   * where it silently swallowed the first click on every other control on the page.
   */
  $effect(() => {
    const element = dialog;
    if (element === null) return;
    const shell = element.closest('.app-shell') ?? document.body;
    if (element.parentElement !== shell) shell.append(element);
    return () => {
      if (element.open) element.close();
      element.remove();
    };
  });

  $effect(() => {
    const element = dialog;
    if (element === null) return;
    if (open && !element.open) {
      element.showModal();
      void focusInitial(element);
    } else if (!open && element.open) {
      element.close();
    }
  });

  async function focusInitial(element: HTMLDialogElement): Promise<void> {
    await tick();
    initialModalFocus(element)?.focus();
  }

  function requestClose(): void {
    onClose();
    queueMicrotask(() => returnFocus?.focus());
  }

  function cancel(event: Event): void {
    event.preventDefault();
    requestClose();
  }

  function keydown(event: KeyboardEvent): void {
    if (event.key !== 'Escape' || event.defaultPrevented) return;
    event.preventDefault();
    requestClose();
  }

  function outside(event: MouseEvent): void {
    if (event.target === dialog) requestClose();
  }
</script>

<dialog
  class:inspector={variant === 'inspector'}
  class:wide={variant === 'wide'}
  bind:this={dialog}
  aria-labelledby={elementIds.title}
  aria-describedby={description === undefined ? undefined : elementIds.description}
  oncancel={cancel}
  onclick={outside}
  onkeydown={keydown}
>
  <section class="modal-panel">
    <header>
      <div class="modal-heading">
        <h2 id={elementIds.title}>{title}</h2>
        {#if description !== undefined}
          <p id={elementIds.description}>{description}</p>
        {/if}
      </div>
      {#if headerExtra !== undefined}
        <span class="header-extra">{@render headerExtra()}</span>
      {/if}
      <!-- The approved dialogs carry no header X — footer buttons and Escape
           close them. The control stays for assistive tech only. -->
      <button
        class="modal-close visually-hidden"
        type="button"
        aria-label={closeLabel}
        onclick={requestClose}
      >
        <Icon name="close" size={18} />
      </button>
    </header>

    <div class="modal-content">
      {@render children()}
    </div>

    {#if footer !== undefined}
      <footer>{@render footer()}</footer>
    {/if}
  </section>
</dialog>

<style>
  dialog {
    background: transparent;
    border: 0;
    color: var(--text);
    height: 100%;
    margin: auto;
    max-height: none;
    max-width: none;
    padding: 1rem;
    width: 100%;
    z-index: var(--layer-dialog);
  }

  dialog::backdrop {
    background: var(--scrim);
    backdrop-filter: blur(8px);
  }

  .modal-panel {
    background: var(--dialog-bg);
    border: 1px solid var(--dialog-border);
    border-radius: var(--radius-dialog);
    box-shadow: var(--shadow-dialog);
    display: grid;
    grid-template-rows: auto minmax(0, 1fr) auto;
    left: 50%;
    max-height: calc(100dvh - 2rem);
    max-width: 34rem;
    overflow: hidden;
    position: absolute;
    top: 50%;
    transform: translate(-50%, -50%);
    width: calc(100% - 2rem);
  }

  dialog.inspector {
    padding: 0;
  }

  .inspector .modal-panel {
    border-block: 0;
    border-radius: 0;
    border-right: 0;
    bottom: 0;
    height: 100dvh;
    left: auto;
    max-height: none;
    max-width: 40rem;
    right: 0;
    top: 0;
    transform: none;
    width: min(40rem, 92vw);
  }

  .wide .modal-panel {
    max-width: 40rem;
  }

  header {
    align-items: flex-start;
    display: flex;
    gap: 1rem;
    justify-content: space-between;
    padding: var(--space-6) var(--space-6) var(--space-3);
  }

  .modal-heading {
    flex: 1;
    min-width: 0;
  }

  /* Rides the title line, like the Inbox count chip in the approved mock. */
  .header-extra {
    align-self: flex-start;
    flex: none;
    margin-top: 0.2rem;
  }

  h2 {
    font-size: 1.25rem;
    font-weight: 700;
    letter-spacing: -0.015em;
    line-height: 1.25;
    margin: 0;
  }

  header p {
    color: var(--dim);
    font-size: 0.8125rem;
    line-height: 1.5;
    margin: 0.3rem 0 0;
  }

  .modal-close {
    background: transparent;
    border: 1px solid transparent;
    border-radius: var(--r-ctl);
    flex: none;
    color: var(--text-secondary);
    height: var(--local-control-height, var(--control-height-compact));
    padding: 0;
    position: absolute;
    width: var(--local-control-height, var(--control-height-compact));
  }

  .modal-close:hover {
    background: var(--strip-lift);
    border-color: var(--control-border);
  }

  /* One stack rhythm for every dialog body. Each dialog used to space its own
     sections with whatever margin it picked, so two dialogs open side by side
     did not agree on the gap between a card and the block under it. */
  .modal-content {
    align-content: start;
    display: grid;
    gap: 0.875rem;
    min-height: 0;
    overflow-y: auto;
    padding: var(--space-3) var(--space-6) var(--space-6);
  }

  /* Global: the children come from the caller, so scoped styles never reach
     them. Grid items default to min-width auto and would refuse to shrink
     below a long unbroken path or login. */
  .modal-content > :global(*) {
    min-width: 0;
  }

  footer {
    align-items: center;
    background: var(--dialog-bg);
    display: flex;
    gap: 0.625rem;
    justify-content: flex-end;
    border-top: 1px solid var(--border-subtle);
    padding: var(--space-4) var(--space-6);
  }

  @media (max-width: 38rem) {
    dialog {
      padding: var(--space-2);
    }

    .modal-panel {
      max-height: calc(100dvh - var(--space-4));
      width: calc(100% - var(--space-4));
    }

    .inspector .modal-panel {
      height: 100dvh;
      max-height: none;
      width: 100%;
    }

    header,
    .modal-content,
    footer {
      padding-left: var(--space-4);
      padding-right: var(--space-4);
    }
  }

  @media (prefers-reduced-motion: no-preference) {
    dialog[open] .modal-panel {
      animation: modal-in var(--duration-normal) var(--ease-out);
    }

    dialog[open].inspector .modal-panel {
      animation: inspector-in var(--duration-normal) var(--ease-out);
    }

    @keyframes modal-in {
      from {
        opacity: 0;
        transform: translate(-50%, calc(-50% + 0.5rem));
      }
    }

    @keyframes inspector-in {
      from {
        opacity: 0;
        transform: translateX(1rem);
      }
    }
  }
</style>

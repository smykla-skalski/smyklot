<script lang="ts">
  import { tick, type Snippet } from 'svelte';
  import { initialModalFocus, modalElementIds } from '../lib/modal';

  const {
    id,
    open,
    title,
    description,
    closeLabel = 'Close dialog',
    returnFocus = null,
    onClose,
    children,
    footer,
  }: {
    id: string;
    open: boolean;
    title: string;
    description?: string;
    closeLabel?: string;
    returnFocus?: HTMLElement | null;
    onClose: () => void;
    children: Snippet;
    footer?: Snippet;
  } = $props();

  let dialog = $state<HTMLDialogElement | null>(null);
  const elementIds = $derived(modalElementIds(id));

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

  function outside(event: MouseEvent): void {
    if (event.target === dialog) requestClose();
  }
</script>

<dialog
  bind:this={dialog}
  aria-labelledby={elementIds.title}
  aria-describedby={description === undefined ? undefined : elementIds.description}
  oncancel={cancel}
  onclick={outside}
>
  <section class="modal-panel">
    <header>
      <div>
        <h2 id={elementIds.title}>{title}</h2>
        {#if description !== undefined}
          <p id={elementIds.description}>{description}</p>
        {/if}
      </div>
      <button class="modal-close" type="button" aria-label={closeLabel} onclick={requestClose}>
        <span aria-hidden="true"></span>
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
  }

  dialog::backdrop {
    background: color-mix(in srgb, var(--bg) 68%, transparent);
    backdrop-filter: blur(8px) saturate(0.72);
  }

  .modal-panel {
    background: var(--strip);
    border: 1px solid var(--control-border);
    border-radius: var(--r-strip);
    box-shadow: 0 24px 64px color-mix(in srgb, var(--shadow) 85%, black);
    left: 50%;
    max-height: calc(100vh - 2rem);
    max-width: 34rem;
    overflow: auto;
    position: absolute;
    top: 50%;
    transform: translate(-50%, -50%);
    width: calc(100% - 2rem);
  }

  header {
    align-items: flex-start;
    display: flex;
    gap: 1rem;
    justify-content: space-between;
    padding: 1.25rem 1.25rem 0.5rem;
  }

  h2 {
    font-size: 1rem;
    line-height: 1.25;
    margin: 0;
  }

  header p {
    color: var(--dim);
    font-size: 0.8125rem;
    line-height: 1.45;
    margin: 0.3rem 0 0;
  }

  .modal-close {
    background: transparent;
    border: 1px solid transparent;
    border-radius: var(--r-ctl);
    flex: none;
    height: 1.875rem;
    padding: 0;
    position: relative;
    width: 1.875rem;
  }

  .modal-close:hover {
    background: var(--strip-lift);
    border-color: var(--control-border);
  }

  .modal-close span::before,
  .modal-close span::after {
    background: var(--dim);
    content: '';
    height: 1px;
    left: 0.45rem;
    position: absolute;
    top: 0.9rem;
    width: 0.875rem;
  }

  .modal-close span::before {
    transform: rotate(45deg);
  }

  .modal-close span::after {
    transform: rotate(-45deg);
  }

  .modal-content {
    padding: 0.75rem 1.25rem 1.25rem;
  }

  footer {
    align-items: center;
    display: flex;
    gap: 0.625rem;
    justify-content: flex-end;
    padding: 0 1.25rem 1.25rem;
  }

  @media (prefers-reduced-motion: no-preference) {
    dialog[open] .modal-panel {
      animation: modal-in 140ms ease-out;
    }

    @keyframes modal-in {
      from {
        opacity: 0;
        transform: translate(-50%, calc(-50% + 0.5rem));
      }
    }
  }
</style>

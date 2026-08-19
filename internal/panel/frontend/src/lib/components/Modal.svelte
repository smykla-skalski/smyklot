<script lang="ts">
  import { type Snippet } from 'svelte';
  import { Dialog } from 'bits-ui';

  const {
    id,
    open,
    title,
    description,
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
    variant?: 'dialog' | 'inspector' | 'wide';
    returnFocus?: HTMLElement | null;
    onClose: () => void;
    children: Snippet;
    footer?: Snippet;
    headerExtra?: Snippet;
  } = $props();

  let openState = $derived(open);

  function handleOpenChange(detail: boolean): void {
    if (!detail) {
      onClose();
      queueMicrotask(() => returnFocus?.focus());
    }
  }
</script>

<Dialog.Root bind:open={openState} onOpenChange={handleOpenChange}>
  <Dialog.Portal to=".app-shell">
    <Dialog.Overlay class="modal-overlay" />
    <div
      class="modal-content-wrapper {variant === 'inspector' ? 'inspector' : ''} {variant === 'wide'
        ? 'wide'
        : ''}"
    >
      <Dialog.Content
        {id}
        class="modal-panel"
        aria-labelledby={`${id}-title`}
        aria-describedby={description === undefined ? undefined : `${id}-description`}
      >
        <header>
          <div class="modal-heading">
            <div class="heading-row">
              <h2 class="band-trim" id={`${id}-title`}>{title}</h2>
              {#if headerExtra !== undefined}
                <span class="header-extra">{@render headerExtra()}</span>
              {/if}
            </div>
            {#if description !== undefined}
              <p id={`${id}-description`}>{description}</p>
            {/if}
          </div>
        </header>

        <div class="modal-body">
          {@render children()}
        </div>

        {#if footer !== undefined}
          <footer>{@render footer()}</footer>
        {/if}
      </Dialog.Content>
    </div>
  </Dialog.Portal>
</Dialog.Root>

<style>
  :global(.modal-overlay) {
    position: fixed;
    inset: 0;
    background: var(--scrim);
    backdrop-filter: blur(8px);
    z-index: var(--layer-dialog);
  }

  :global(.modal-content-wrapper) {
    background: transparent;
    border: 0;
    color: var(--text);
    inset: 0;
    margin: auto;
    max-height: none;
    max-width: none;
    padding: 1rem;
    pointer-events: none;
    position: fixed;
    width: 100%;
    z-index: var(--layer-dialog);
  }

  :global(.modal-panel) {
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
    pointer-events: auto;
    position: absolute;
    top: 50%;
    transform: translate(-50%, -50%);
    width: calc(100% - 2rem);
  }

  :global(.modal-content-wrapper.inspector) {
    padding: 0;
  }

  :global(.inspector .modal-panel) {
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

  :global(.wide .modal-panel) {
    max-width: 40rem;
  }

  :global(.modal-panel header) {
    align-items: flex-start;
    display: flex;
    gap: 1rem;
    justify-content: space-between;
    padding: var(--space-6) var(--space-6) var(--space-3);
  }

  :global(.modal-heading) {
    flex: 1;
    min-width: 0;
  }

  :global(.heading-row) {
    align-items: center;
    display: flex;
    gap: 1rem;
    justify-content: space-between;
    min-width: 0;
  }

  :global(.header-extra) {
    flex: none;
  }

  :global(.modal-panel h2) {
    font-size: 1.25rem;
    font-weight: 700;
    letter-spacing: -0.015em;
    line-height: 1.25;
    margin: 0;
    min-width: 0;
    overflow-wrap: anywhere;
  }

  :global(.modal-panel header p) {
    color: var(--dim);
    font-size: 0.8125rem;
    line-height: 1.5;
    margin: 0.45rem 0 0;
  }

  :global(.modal-body) {
    align-content: start;
    display: grid;
    gap: 0.875rem;
    min-height: 0;
    overflow-y: auto;
    padding: var(--space-3) var(--space-6) var(--space-6);
  }

  :global(.modal-body > *) {
    min-width: 0;
  }

  :global(.modal-panel footer) {
    align-items: center;
    background: var(--dialog-bg);
    display: flex;
    gap: 0.625rem;
    justify-content: flex-end;
    border-top: 1px solid var(--border-subtle);
    padding: var(--space-4) var(--space-6);
  }

  @media (max-width: 38rem) {
    :global(.modal-content-wrapper) {
      padding: var(--space-2);
    }

    :global(.modal-panel) {
      max-height: calc(100dvh - var(--space-4));
      width: calc(100% - var(--space-4));
    }

    :global(.inspector .modal-panel) {
      height: 100dvh;
      max-height: none;
      width: 100%;
    }

    :global(.modal-panel header),
    :global(.modal-body),
    :global(.modal-panel footer) {
      padding-left: var(--space-4);
      padding-right: var(--space-4);
    }
  }

  @media (prefers-reduced-motion: no-preference) {
    :global(.modal-panel) {
      animation: modal-in var(--duration-normal) var(--ease-out);
    }

    :global(.inspector .modal-panel) {
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

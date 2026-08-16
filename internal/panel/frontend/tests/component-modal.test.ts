// @vitest-environment jsdom
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import Modal from '../src/lib/components/Modal.svelte';

describe('Modal [Component]', () => {
  beforeEach(() => {
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  it('uses dialog semantics and reports Escape as a close request', async () => {
    const onClose = vi.fn();
    const returnFocus = document.createElement('button');
    returnFocus.textContent = 'Open settings';
    document.body.append(returnFocus);

    render(Modal, {
      target: document.querySelector('.app-shell') as HTMLElement,
      props: {
        id: 'settings-dialog',
        open: true,
        title: 'Repository settings',
        description: 'Override inherited settings',
        returnFocus,
        onClose,
        children: createRawSnippet(() => ({ render: () => '<p>Settings body</p>' })),
      },
    });

    const dialog = screen.getByRole('dialog', { name: 'Repository settings' });
    expect(dialog.classList.contains('modal-panel')).toBe(true);
    expect(dialog.parentElement?.classList.contains('modal-content-wrapper')).toBe(true);
    expect(screen.getByText('Override inherited settings')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Close dialog' })).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(dialog));

    await fireEvent.keyDown(document, { key: 'Escape' });

    expect(onClose).toHaveBeenCalledOnce();
    await waitFor(() => expect(document.activeElement).toBe(returnFocus));
  });
});

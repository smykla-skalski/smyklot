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
  it('keeps its mounted content and focus when an incidental close is blocked', async () => {
    const onClose = vi.fn();
    const beforeClose = vi.fn(() => false);
    const rendered = render(Modal, {
      id: 'guarded-dialog',
      open: true,
      title: 'Private edit',
      onClose,
      beforeClose,
      children: createRawSnippet(() => ({ render: () => '<input aria-label="Private value" />' })),
    });
    const input = screen.getByRole('textbox', { name: 'Private value' });
    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByRole('dialog', { name: 'Private edit' })),
    );
    input.focus();
    await fireEvent.input(input, { target: { value: 'unfinished' } });
    await fireEvent.keyDown(document, { key: 'Escape' });
    expect(beforeClose).toHaveBeenCalledOnce();
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole('textbox', { name: 'Private value' })).toBe(input);
    expect(document.activeElement).toBe(input);
    expect((input as HTMLInputElement).value).toBe('unfinished');

    await rendered.rerender({ beforeClose: () => true });
    await fireEvent.keyDown(document, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledOnce();
  });
});

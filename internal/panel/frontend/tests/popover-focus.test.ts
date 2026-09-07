// @vitest-environment jsdom
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import PopoverFocusHarness from './support/PopoverFocusHarness.svelte';

describe('Popover close focus [Component]', () => {
  beforeEach(() => {
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  it('restores its trigger by default', async () => {
    render(PopoverFocusHarness);
    const trigger = screen.getByRole('button', { name: 'Open choices' });
    trigger.focus();
    await fireEvent.click(trigger);
    await fireEvent.click(await screen.findByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });

  it('lets the caller replace restoration through the native cancellable event', async () => {
    const onCloseAutoFocus = vi.fn((event: Event) => {
      event.preventDefault();
      screen.getByRole('textbox', { name: 'Focus destination' }).focus();
    });
    render(PopoverFocusHarness, { onCloseAutoFocus });
    await fireEvent.click(screen.getByRole('button', { name: 'Open choices' }));
    await fireEvent.click(await screen.findByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(onCloseAutoFocus).toHaveBeenCalledOnce());
    expect(onCloseAutoFocus.mock.calls[0]?.[0].defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(screen.getByRole('textbox', { name: 'Focus destination' }));
  });
});

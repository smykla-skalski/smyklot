import { describe, expect, it, vi } from 'vitest';

import { initialModalFocus, modalElementIds } from '../src/lib/modal';

describe('modalElementIds', () => {
  it('keeps each dialog title and description reference unique', () => {
    expect(modalElementIds('add-user')).toEqual({
      title: 'add-user-title',
      description: 'add-user-description',
    });
    expect(modalElementIds('user-action')).not.toEqual(modalElementIds('add-user'));
  });
});

describe('initialModalFocus', () => {
  it('prefers the field explicitly chosen by the modal content', () => {
    const preferred = {} as HTMLElement;
    const fallback = {} as HTMLElement;
    const querySelector = vi.fn((selector: string) =>
      selector === '[data-modal-focus]' ? preferred : fallback,
    );

    expect(initialModalFocus({ querySelector } as unknown as ParentNode)).toBe(preferred);
    expect(querySelector).toHaveBeenCalledOnce();
  });

  it('falls back to the first form control when no preferred field exists', () => {
    const fallback = {} as HTMLElement;
    const querySelector = vi.fn((selector: string) =>
      selector === '[data-modal-focus]' ? null : fallback,
    );

    expect(initialModalFocus({ querySelector } as unknown as ParentNode)).toBe(fallback);
    expect(querySelector).toHaveBeenCalledTimes(2);
  });
});

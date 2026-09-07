// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import DiffBlock from '../src/lib/components/DiffBlock.svelte';

describe('folded conflict comparison', () => {
  it('keeps the context toggle focused and resets expansion for a fresh comparison', async () => {
    const props = {
      before: '"value": "panel"\nunchanged1\nunchanged2\n',
      after: '"value": "file"\nunchanged1\nunchanged2\n',
      contextLines: 0,
      label: 'Conflicting settings',
      labels: { before: 'Panel values', after: 'File values' },
    };
    const component = render(DiffBlock, { props });
    const toggle = screen.getByRole('button', { name: 'Show 2 unchanged lines' });
    expect(screen.getByRole('region', { name: 'Conflicting settings' }).textContent).not.toContain(
      'unchanged1',
    );
    toggle.focus();
    await fireEvent.click(toggle);
    expect(document.activeElement).toBe(toggle);
    expect(toggle.getAttribute('aria-expanded')).toBe('true');
    expect(screen.getByRole('region', { name: 'Conflicting settings' }).textContent).toContain(
      'unchanged1',
    );
    expect(document.getElementById(toggle.getAttribute('aria-controls')!)?.hidden).toBe(false);
    await fireEvent.click(toggle);
    expect(toggle.textContent).toContain('Show 2 unchanged lines');
    expect(document.activeElement).toBe(toggle);
    expect(screen.getByRole('region', { name: 'Conflicting settings' }).textContent).not.toContain(
      'unchanged1',
    );
    await fireEvent.click(toggle);
    await component.rerender({ ...props, after: '"value": "updated"\nunchanged1\nunchanged2\n' });
    expect(screen.getByRole('button', { name: 'Show 2 unchanged lines' })).toBeTruthy();
    expect(screen.getByRole('region', { name: 'Conflicting settings' }).textContent).not.toContain(
      'unchanged1',
    );
    expect(document.querySelector('.is-del .word')?.textContent).toBe('panel');
    expect(document.querySelector('.is-add .word')?.textContent).toBe('updated');
  });
});

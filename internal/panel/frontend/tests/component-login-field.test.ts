// @vitest-environment jsdom
import { render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it } from 'vitest';

import LoginField from '../src/lib/components/LoginField.svelte';

describe('LoginField [Unit]', () => {
  beforeEach(() => {
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  it('connects a refused value to its explanation', () => {
    render(LoginField, {
      id: 'login',
      value: 'bart',
      label: 'GitHub login',
      refused: true,
      help: 'You cannot change your own access',
      suggest: async () => [],
    });

    const input = screen.getByRole('combobox', { name: 'GitHub login' });
    const help = screen.getByText('You cannot change your own access');

    expect(input.getAttribute('aria-invalid')).toBe('true');
    expect(input.getAttribute('aria-describedby')).toBe(help.id);
    expect(help.id).toBe('login-help');
  });
});

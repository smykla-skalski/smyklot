// @vitest-environment jsdom
import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';

import type { PanelApi } from '../src/lib/api';
import ErrorPage from '../src/lib/components/ErrorPage.svelte';

describe('ErrorPage [Component]', () => {
  it('fits a route failure into the signed-in panel shell', () => {
    const { container } = render(ErrorPage, {
      api: { signInUrl: () => '/auth/sign-in' } as PanelApi,
      base: '',
      build: { version: null, serviceHost: null },
      failure: { status: 404, code: '', message: 'Not found' },
      insidePanel: true,
    });

    expect(screen.getByRole('heading', { name: 'Not found', level: 2 })).toBeTruthy();
    expect(container.querySelector('.panel-error')).toBeTruthy();
    expect(container.querySelector('main')).toBeNull();
    expect(container.querySelector('footer')).toBeNull();
  });
});

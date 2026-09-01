// @vitest-environment jsdom
import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';

import type { PanelApi } from '../src/lib/api';
import ErrorPage from '../src/lib/components/ErrorPage.svelte';

const api = { signInUrl: () => '/auth/sign-in' } as PanelApi;
const build = { version: null, serviceHost: null };

describe('ErrorPage [Component]', () => {
  /**
   * A reader who is already inside is on a PAGE, not on an error screen: the shell is
   * still around them, so the failure is said in the page's own voice - a head, a card
   * and somewhere that does exist - rather than as the centred plate and five-rem status
   * number a reader arriving cold is given.
   */
  it('answers a route failure inside the shell as a page', () => {
    const { container } = render(ErrorPage, {
      api,
      base: '',
      build,
      failure: { status: 404, code: '', message: 'Not found' },
      insidePanel: true,
      destinations: [
        { label: 'Repositories', href: '/workspace/acme/repositories' },
        { label: 'Queue', href: '/workspace/acme/queue' },
      ],
    });

    expect(screen.getByRole('heading', { name: 'Not found', level: 1 })).toBeTruthy();
    expect(container.querySelector('.card .state-panel')).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Repositories' })).toBeTruthy();
    // The shell around it owns the landmarks and the footer.
    expect(container.querySelector('main')).toBeNull();
    expect(container.querySelector('footer')).toBeNull();
    // No status number: the sentence says what happened, and a reader inside knows where
    // they are without being handed three digits.
    expect(container.querySelector('.error-code')).toBeNull();
  });

  it('offers nothing elsewhere when the session names nowhere to go', () => {
    const { container } = render(ErrorPage, {
      api,
      base: '',
      build,
      failure: { status: 404, code: '', message: 'Not found' },
      insidePanel: true,
    });

    expect(container.querySelector('.error-elsewhere')).toBeNull();
  });
});

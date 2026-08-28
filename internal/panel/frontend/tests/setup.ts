import { act, cleanup, setup } from '@testing-library/svelte';
import { beforeEach } from 'vitest';

const BODY_UNLOCK_DELAY_MS = 30;

function bodyScrollIsLocked(): boolean {
  if (typeof document === 'undefined') return false;
  return document.body.style.overflow === 'hidden' || document.body.style.pointerEvents === 'none';
}

beforeEach(async () => {
  await setup();

  return async () => {
    await act();
    const waitForBodyUnlock = bodyScrollIsLocked();
    cleanup();

    if (waitForBodyUnlock) {
      // Bits UI deliberately hands body-scroll ownership over through a 24 ms
      // timer. Keep jsdom alive until that callback restores document.body.
      await new Promise((resolve) => setTimeout(resolve, BODY_UNLOCK_DELAY_MS));
    }
  };
});

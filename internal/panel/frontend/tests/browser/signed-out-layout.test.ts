import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { startPanel, type Panel } from './harness';

/**
 * A reader who is not signed in never sees the panel.
 *
 * Whether an address shows the application or the sign-in page is answered by a
 * request, and the layout used to choose before the answer arrived: `signedOut`
 * is `!loading && viewer === null`, which is false while the question is still
 * outstanding, so every load fell through to the shell. A reader who turned out
 * to be signed out watched a sidebar, a workspace and a footer draw themselves
 * and then be taken away and replaced by a sign-in form.
 *
 * It is measured as a film rather than as an end state, because the end state
 * was always right - the sign-in page did arrive. What was wrong only existed
 * for a few hundred milliseconds, which is exactly long enough to see.
 */

/** Slow enough that the window between question and answer can be sampled. */
const ANSWER_DELAY_MS = 900;
const SAMPLES = 30;
const SAMPLE_EVERY_MS = 100;

let panel: Panel;
let drawn: string[] = [];

beforeAll(async () => {
  panel = await startPanel();

  const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
  try {
    /* What being signed out actually is on the wire. The mock has a scenario for
       it, but it re-reads the scenario on every document request and defaults
       back to signed in, so a deep link cannot hold one. */
    await page.route('**/api/v1/session', async (route) => {
      await new Promise((resolve) => setTimeout(resolve, ANSWER_DELAY_MS));
      await route.fulfill({
        status: 401,
        contentType: 'application/json',
        body: '{"error":"unauthenticated"}',
      });
    });

    await page.goto(`${panel.origin}/i/${panel.account}/repositories`, {
      waitUntil: 'domcontentloaded',
    });

    const seen: string[] = [];
    for (let sample = 0; sample < SAMPLES; sample += 1) {
      seen.push(
        await page.evaluate(() => {
          const shell = document.querySelector('.app-shell') !== null;
          const boot = document.querySelector('.panel-boot') !== null;
          const signIn = /sign in with github|continue with github/iu.test(document.body.innerText);

          return (
            [boot && 'boot', shell && 'shell', signIn && 'sign-in'].filter(Boolean).join('+') ||
            'blank'
          );
        }),
      );
      await page.waitForTimeout(SAMPLE_EVERY_MS);
    }

    // Collapsed to the sequence of distinct states, which is what a reader sees.
    drawn = seen.filter((state, index) => state !== seen[index - 1]);
  } finally {
    await page.close();
  }
}, 300_000);

afterAll(async () => {
  await panel?.close();
});

describe('opening the panel while signed out [Integration]', () => {
  it('reaches the sign-in page', () => {
    // Without this the check below passes on a page that drew nothing at all.
    expect(drawn.at(-1), `what was drawn: ${drawn.join(' -> ')}`).toContain('sign-in');
  });

  it('never draws the application on the way there', () => {
    expect(
      drawn.filter((state) => state.includes('shell')),
      `what was drawn: ${drawn.join(' -> ')}`,
    ).toEqual([]);
  });
});

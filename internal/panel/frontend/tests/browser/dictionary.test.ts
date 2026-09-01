import type { Page } from 'playwright-core';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { addressOf, inLanes, PANEL_ROUTES, startPanel, visit, type Panel } from './harness';

/**
 * One term per concept, enforced on what the browser actually draws.
 *
 * The design system's language view is a contract: for every concept it names the one
 * word that ships and lists the ones it retires. The retired words are not synonyms a
 * reader can absorb - each of them was the reason somebody could not find the page they
 * were on, so a page that still says one is a page the dictionary does not cover.
 *
 * Read from the rendered page rather than from the source, because a word reaches a
 * reader through four routes that no single grep sees together: a literal in a
 * component, a label composed at runtime, a message the Go API writes, and the fixtures
 * the dev mock serves. The sweep that found the last of these found them in that order.
 *
 * Identifiers are not copy and are deliberately out of scope: the wire still spells an
 * elevation `elevation` and a workspace `installation`, and renaming a column is a
 * migration rather than a rewrite. What is measured is text a person reads - the
 * document's own text, and the labels behind a control's accessible name.
 */

/** A word that no longer ships, and the word that replaced it. */
interface Retired {
  /** What to look for, on a word boundary and in any case. */
  pattern: RegExp;
  /** What ships instead, printed with the failure so the fix is in the report. */
  instead: string;
  /**
   * Where the word is still a fact rather than a term.
   *
   * GitHub's own names survive as facts: an installation id is what GitHub calls the
   * grant, so "GitHub installation #3001" is a fact line and stays. Everything else
   * carrying the word is the retired term.
   */
  exceptFor?: RegExp;
}

const RETIRED: readonly Retired[] = [
  {
    pattern: /\binstallations?\b/iu,
    instead: 'workspace',
    exceptFor: /GitHub installation/iu,
  },
  { pattern: /\belevations?\b|\belevated\b/iu, instead: 'operator visit' },
  { pattern: /\bsuper root\b|\broot mode\b|\bsystem-level\b/iu, instead: 'operator' },
  { pattern: /\bowner access\b/iu, instead: 'the owner role' },
  { pattern: /\bworkloads?\b/iu, instead: 'job' },
  { pattern: /\bcadence\b/iu, instead: "the job's rhythm" },
  { pattern: /\bexecution windows?\b/iu, instead: 'hours' },
  { pattern: /\blanes?\b/iu, instead: 'job' },
  { pattern: /\beligible\b/iu, instead: 'what it will act on, said plainly' },
  { pattern: /\benablement\b/iu, instead: 'on / off' },
  { pattern: /\bdeployment configuration\b/iu, instead: 'the deployment' },
  { pattern: /\bglobal polic(?:y|ies)\b/iu, instead: 'the service settings' },
  { pattern: /\bstable passing\b|\bpassing observations?\b/iu, instead: 'quiet period' },
  { pattern: /\bleaves? alone\b/iu, instead: 'ignored' },
  { pattern: /\bstep around\b/iu, instead: 'bypass list' },
  { pattern: /\bpath index\b/iu, instead: 'file index' },
  { pattern: /\bdelivery health\b|\bwebhook deliveries\b/iu, instead: 'failures' },
  { pattern: /\bunconfigured\b/iu, instead: 'the repositories Smyklot is off in' },
  { pattern: /\bretryable\b/iu, instead: 'retrying' },
];

interface Found {
  route: string;
  term: string;
  instead: string;
  where: string;
  text: string;
}

/**
 * Every folded card on the page, opened.
 *
 * A fold is closed markup, so its copy is invisible and this sweep would skip it - and
 * the folds are exactly where the settings pages keep the sentences a reader only meets
 * when they go looking. Opened rather than pressed: a `<details>` needs no animation to
 * be read, and a press per fold is a second's wait per card.
 */
async function openFolds(page: Page): Promise<void> {
  await page.evaluate(() => {
    for (const fold of document.querySelectorAll('details')) fold.open = true;
  });
}

/** A surface a route holds behind a control, and the press that brings it up. */
const BEHIND_A_CONTROL = [
  { route: 'root/schedules', press: 'button:has-text("Edit schedule")', opens: '#policy-editor' },
  /* An inspector fetches the rest of its record after it opens, so the dialog being up
     is not the dialog being readable. */
  { route: 'root/queue', press: 'button.row-hit', opens: '#queue-detail', ready: '.facts' },
  { route: 'root/queue', press: 'button:has-text("Run now")', opens: '#queue-action' },
  {
    route: 'workspace/settings',
    press: 'button:has-text("Request a change")',
    opens: '#workspace-timing-request',
  },
] as const;

function hitsIn(said: { where: string; text: string }[], route: string): Found[] {
  const hits: Found[] = [];

  for (const { where, text } of said) {
    for (const retired of RETIRED) {
      const match = retired.pattern.exec(text);
      if (match === null) continue;
      if (retired.exceptFor?.test(text) === true) continue;
      hits.push({
        route,
        term: match[0],
        instead: retired.instead,
        where,
        text: text.slice(0, 90),
      });
    }
  }

  return hits;
}

function say(hit: Found): string {
  return `${hit.route} ${hit.where}: "${hit.text}" - ${hit.term} -> ${hit.instead}`;
}

/**
 * Every string this page shows a reader, each with the element that holds it.
 *
 * The innermost holder, because every ancestor holds the same words and a match on the
 * shell would otherwise be reported once per level of it. Accessible names are read from
 * the attributes that carry them - a control whose only wording is its `aria-label` is
 * still wording somebody meets, and the icon-only ones are exactly where a retired term
 * survived longest.
 */
async function wordsOn(page: Page, within = ':root'): Promise<{ where: string; text: string }[]> {
  return page.evaluate((root: string) => {
    const label = (element: Element): string => {
      const classes = [...element.classList]
        .filter((one) => !one.startsWith('svelte-'))
        .slice(0, 2)
        .map((one) => `.${one}`)
        .join('');
      return `${element.tagName.toLowerCase()}${classes}`;
    };

    const said: { where: string; text: string }[] = [];
    const holder = document.querySelector(root);
    if (holder === null) return said;

    for (const element of holder.querySelectorAll<HTMLElement>('*')) {
      if (!element.checkVisibility()) continue;

      for (const attribute of ['aria-label', 'title', 'placeholder', 'alt']) {
        const value = element.getAttribute(attribute);
        if (value !== null && value.trim() !== '') {
          said.push({ where: `${label(element)}[${attribute}]`, text: value });
        }
      }

      const text = (element.textContent ?? '').replaceAll(/\s+/gu, ' ').trim();
      if (text === '') continue;
      if ([...element.children].some((child) => (child.textContent ?? '').trim() !== '')) continue;
      said.push({ where: label(element), text });
    }

    return said;
  }, within);
}

let panel: Panel;

beforeAll(async () => {
  panel = await startPanel();
}, 120_000);

afterAll(async () => {
  await panel?.close();
});

describe('The dictionary [Browser]', () => {
  it('is the only vocabulary any page speaks', async () => {
    const found = (
      await inLanes(PANEL_ROUTES, async (route): Promise<Found[]> => {
        const page = await panel.browser.newPage({ viewport: { width: 1440, height: 960 } });
        try {
          await visit(page, addressOf(panel, route));
          await openFolds(page);

          return hitsIn(await wordsOn(page), route);
        } finally {
          await page.close();
        }
      })
    ).flat();

    /* One line per distinct sentence: the shell is on every route, so a word in the
       sidebar is otherwise reported thirty times and the page that owns it is lost in
       the repetition. */
    const seen = new Map<string, Found>();
    for (const hit of found) {
      const key = `${hit.term.toLowerCase()}|${hit.text}`;
      if (!seen.has(key)) seen.set(key, hit);
    }

    expect([...seen.values()].map(say)).toEqual([]);
  }, 240_000);

  /**
   * And so does everything a page keeps behind a control.
   *
   * A dialog is mounted when it opens, so the sweep above cannot see one: the whole of
   * the schedules editor - "Configure workload", "Cadence in seconds", "Execution
   * window" - sat unported behind one press while every route it belongs to measured
   * clean. Opened by the control a reader presses rather than by an address, because
   * these four have none.
   */
  it('is the vocabulary behind a control too', async () => {
    const opened = await inLanes(
      BEHIND_A_CONTROL,
      async (place): Promise<{ place: string; said: number; hits: Found[] }> => {
        const page = await panel.browser.newPage({ viewport: { width: 1440, height: 960 } });
        try {
          await visit(page, addressOf(panel, place.route));
          await openFolds(page);
          await page.locator(place.press).first().click();
          const dialog = page.locator(place.opens).first();
          await dialog.waitFor({ state: 'visible' });
          if ('ready' in place) await dialog.locator(place.ready).first().waitFor();
          /* Read INSIDE the dialog, so a press that opened nothing is a count of zero
             rather than a page that happens to be clean. */
          const said = await wordsOn(page, place.opens);

          return { place: place.opens, said: said.length, hits: hitsIn(said, place.route) };
        } finally {
          await page.close();
        }
      },
    );

    expect(opened.map((one) => `${one.place}: ${one.said > 4 ? 'read' : 'read nothing'}`)).toEqual(
      BEHIND_A_CONTROL.map((place) => `${place.opens}: read`),
    );
    expect(opened.flatMap((one) => one.hits).map(say)).toEqual([]);
  }, 240_000);

  /**
   * One relative time per row, and the whole instant always within reach.
   *
   * The other half of the same contract: a row says how long ago once, and the exact
   * stamp with its timezone is somewhere a reader can get to. The panel spells the
   * second half as a press rather than as the `title` the language view names -
   * `RelativeTime` says why, and the reasons are a phone and a keyboard - so what is
   * measured is reachability, which either spelling satisfies.
   *
   * Two relative times in one row is the failure the rule exists for: a row carrying
   * both when a thing happened and when it will next happen makes the reader work out
   * which of the two "in 4 minutes" belongs to.
   */
  it('shows one relative time per row, and the whole instant is reachable', async () => {
    const found = (
      await inLanes(PANEL_ROUTES, async (route): Promise<string[]> => {
        const page = await panel.browser.newPage({ viewport: { width: 1440, height: 960 } });
        try {
          await visit(page, addressOf(panel, route));
          await openFolds(page);

          return (
            await page.evaluate(() => {
              const said: string[] = [];
              const ROWS =
                '.object-row, .repo-row, .policy-row, .label-row, .data-row, .attn-row, tbody tr';

              for (const row of document.querySelectorAll<HTMLElement>(ROWS)) {
                const times = [...row.querySelectorAll('time')].filter((one) =>
                  one.checkVisibility(),
                );
                const words = (row.textContent ?? '').replaceAll(/\s+/gu, ' ').trim().slice(0, 70);
                /* A row inside a row is one row: the outer holds the inner's times too. */
                if (row.querySelector(ROWS) === null && times.length > 1) {
                  said.push(`${times.length} relative times in one row: "${words}"`);
                }
                for (const time of times) {
                  const reachable =
                    time.hasAttribute('data-exact') ||
                    time.getAttribute('title') !== null ||
                    time.closest('[title]') !== null;
                  if (!reachable) said.push(`no exact instant behind "${time.textContent}"`);
                }
              }

              return said;
            })
          ).map((one) => `${route} ${one}`);
        } finally {
          await page.close();
        }
      })
    ).flat();

    expect([...new Set(found)]).toEqual([]);
  }, 240_000);
});

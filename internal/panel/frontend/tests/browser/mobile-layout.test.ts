/**
 * That every page fits the phone it is being read on.
 *
 * The symptom this exists for is not a sideways scrollbar. A page whose content
 * demands more width than the device has does not simply overflow: Chrome widens
 * the *layout viewport* to whatever the content asked for and scales the page
 * down to fit the screen, so the reader gets no scrollbar and no clipped edge -
 * they get every glyph on the page shrunk. The Root console rendered at 75% on a
 * 320px screen because one row of three buttons would not wrap, and the settings
 * page at 85% because a segmented control holds a fixed 157px and the label
 * beside it was the only part that could give.
 *
 * That makes `window.innerWidth` the measurement worth taking, and it has to be
 * compared against the width the device was *given*, not against itself. Reading
 * anything else against `innerWidth` is how this went unnoticed: every edge test
 * of the form "is this element past the right edge" passes once the browser has
 * moved the right edge out to meet it.
 *
 * `document.scrollWidth` is no good either - once the viewport has been widened
 * the document fits inside it exactly, so the page reports itself as fitting.
 */
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { SETTLE_MS, startPanel, type Panel } from './harness';

/** The narrowest phone still in use, and the common one. */
const WIDTHS = [320, 375] as const;

interface Measured {
  /** What the page decided it needed. Equal to the device width when it fits. */
  layoutViewport: number;
  /** Proof the route rendered: its heading, so a 404 cannot pass this file. */
  heading: string | null;
  /** Named only when it does not fit, so a failure says what to go and look at. */
  widest: { right: number; element: string; text: string } | null;
  /** Controls the page renders and a thumb cannot land on. */
  unreachable: string[];
}

let panel: Panel;
const measured = new Map<string, Measured>();

/** Every addressable page, named as the reader would say it. */
function routes(account: string): ReadonlyArray<readonly [string, string]> {
  return [
    ['settings', `/i/${account}/settings`],
    ['repositories', `/i/${account}/repositories`],
    ['users', `/i/${account}/users`],
    ['invitations', `/i/${account}/invitations`],
    ['audit history', `/i/${account}/history/audit`],
    ['failure history', `/i/${account}/history/failures`],
    ['the inbox', `/inbox`],
    ['the Root overview', `/root`],
    ['the installation catalog', `/root/installations`],
    ['Root access users', `/root/access/users`],
    ['Root access invitations', `/root/access/invitations`],
    ['Root audit history', `/root/history/audit`],
    ['Root failure history', `/root/history/failures`],
    ['Root settings', `/root/settings`],
    ['a Root installation', `/root/installations/${account}/settings`],
    ['a Root installation’s repositories', `/root/installations/${account}/repositories`],
  ] as const;
}

beforeAll(async () => {
  panel = await startPanel();

  for (const width of WIDTHS) {
    for (const [name, path] of routes(panel.account)) {
      measured.set(`${name} at ${width}`, await measure(path, width));
    }
  }
}, 300_000);

afterAll(async () => {
  await panel?.close();
});

async function measure(path: string, width: number): Promise<Measured> {
  /* Emulated as a phone rather than merely sized like one. The viewport widening
     this looks for is mobile behaviour: a desktop Chrome at 320px scrolls
     sideways instead, which is a different symptom and would not be caught. */
  const page = await panel.browser.newPage({
    viewport: { width, height: 812 },
    deviceScaleFactor: 2,
    isMobile: true,
    hasTouch: true,
  });

  try {
    await page.goto(`${panel.origin}${path}`, { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(SETTLE_MS);

    return await page.evaluate((device: number) => {
      const describe_ = (element: Element): string => {
        const classes =
          typeof element.className === 'string'
            ? element.className
                .split(/\s+/u)
                .filter((name) => name !== '' && !name.startsWith('svelte-'))
                .join('.')
            : '';

        return `${element.tagName.toLowerCase()}${classes === '' ? '' : `.${classes}`}`;
      };

      /* Finding the culprit needs the page laid out at the width it was supposed
         to have. In the widened viewport nothing is locally wrong - the browser
         gave the content everything it asked for, so every box fits its parent
         and the only thing geometry can report is that the outermost ones reach
         the far edge. Pinning the root back to the device width lays it out
         again under the real constraint, and then whatever still sticks out past
         that edge is the thing that would not shrink. The deepest one is the
         content itself rather than the boxes carrying it. */
      let widest: { right: number; element: string; text: string } | null = null;
      if (window.innerWidth > device) {
        const root = document.documentElement;
        const pinned = root.style.cssText;
        root.style.width = `${device}px`;
        root.style.overflowX = 'hidden';
        root.getBoundingClientRect();

        let depth = -1;
        for (const element of document.querySelectorAll('body *')) {
          const rect = element.getBoundingClientRect();
          if (rect.width === 0 || rect.height === 0 || rect.right <= device + 1) continue;

          let own = 0;
          for (let node = element.parentElement; node !== null; node = node.parentElement) own += 1;
          if (own <= depth) continue;

          depth = own;
          widest = {
            right: Math.round(rect.right),
            element: describe_(element),
            text: (element.textContent ?? '').replace(/\s+/gu, ' ').trim().slice(0, 60),
          };
        }
        root.style.cssText = pinned;
      }

      /* A control that is on the page and cannot be pressed. The status filter
         on the users and invitations tables was one: the mobile rule hid every
         heading that carried no sort button, and Status carries a funnel
         instead, so the whole heading went into a 1px box - filter and all -
         while staying in the page and in the tab order. Nothing that measures
         overflow would ever see it, because a control crushed to nothing
         overflows nothing. */
      const unreachable: string[] = [];
      const CONTROLS =
        'button, a[href], input:not([type=hidden]), select, textarea, [role="button"], [role="tab"], [role="switch"], [role="menuitem"]';
      for (const control of document.querySelectorAll(CONTROLS)) {
        // Shut, rather than lost: a closed popover, dialog or drawer collapses
        // its contents on purpose, and `checkVisibility` reads the ancestors.
        if (!control.checkVisibility()) continue;
        if (control.closest('[popover]:not(:popover-open)') !== null) continue;
        if (control.closest('dialog:not([open])') !== null) continue;
        if (control.closest('[inert], [aria-hidden="true"], [hidden]') !== null) continue;
        // Offered to a screen reader alone, which is a decision rather than a
        // defect - as is a radio sitting 1x1 under the label that operates it.
        if (control.closest('.visually-hidden') !== null) continue;
        if (control.tagName === 'INPUT' && control.closest('label') !== null) continue;

        const box = control.getBoundingClientRect();
        let crushed: Element | null = null;
        for (let node = control.parentElement; node !== null; node = node.parentElement) {
          const around = node.getBoundingClientRect();
          if (around.width > 8 && around.height > 8) continue;
          if (getComputedStyle(node).overflow === 'visible') continue;
          crushed = node;
          break;
        }
        if (box.width >= 8 && box.height >= 8 && crushed === null) continue;

        const named = (control.getAttribute('aria-label') ?? control.textContent ?? '')
          .replace(/\s+/gu, ' ')
          .trim()
          .slice(0, 40);
        unreachable.push(
          `${describe_(control)} ${Math.round(box.width)}x${Math.round(box.height)}` +
            `${crushed === null ? '' : ` inside ${describe_(crushed)}`} "${named}"`,
        );
      }

      return {
        layoutViewport: window.innerWidth,
        heading: document.querySelector('h1, h2')?.textContent?.trim() ?? null,
        widest,
        unreachable: [...new Set(unreachable)],
      };
    }, width);
  } finally {
    await page.close();
  }
}

const cases = WIDTHS.flatMap((width) =>
  routes('').map(([name]) => [`${name} at ${width}`, width] as const),
);

describe('every page on a phone', () => {
  it.each(cases)('renders %s', (key) => {
    const result = measured.get(key);
    if (result === undefined) throw new Error(`${key} was never measured`);

    /* The precondition, and not a formality: an address that does not resolve
       renders the 404 page, which is a short centred card that fits any width.
       Without this, a mistyped route here would pass while measuring nothing -
       which is exactly how the first sweep of these pages reported the Root
       console as clean. */
    expect(result.heading, `${key} rendered no heading at all`).not.toBeNull();
    expect(result.heading, `${key} rendered the error page instead of the route`).not.toBe(
      'Not found',
    );
  });

  it.each(cases)('fits %s without shrinking the page', (key, width) => {
    const result = measured.get(key);
    if (result === undefined) throw new Error(`${key} was never measured`);

    expect(
      result.layoutViewport,
      `${key} needed ${result.layoutViewport}px, so the browser zoomed the page to ` +
        `${Math.round((width / result.layoutViewport) * 100)}%. The widest thing on it is ` +
        `${result.widest?.element ?? 'unknown'} reaching ${result.widest?.right ?? 0}px: ` +
        `"${result.widest?.text ?? ''}"`,
    ).toBe(width);
  });

  it.each(cases)('leaves nothing on %s that cannot be pressed', (key) => {
    const result = measured.get(key);
    if (result === undefined) throw new Error(`${key} was never measured`);

    expect(
      result.unreachable,
      `${key} renders controls a thumb cannot land on:\n  ${result.unreachable.join('\n  ')}`,
    ).toEqual([]);
  });
});

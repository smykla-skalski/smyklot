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

import { inLanes, startPanel, visit, type Panel } from './harness';

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
  /** Column-heading filters that are on the page but hidden at this width. */
  hiddenFilters: number;
  /** Filters the reader can actually open here, from a heading or from the tools menu. */
  reachableFilters: number;
}

/** The bar's controls, measured by what a thumb can hit rather than what is painted. */
interface Target {
  control: { width: number; height: number };
  /** The pressable area, which is the overlay when there is one. */
  area: { width: number; height: number; onScreen: boolean };
  /** Anything else pressable the area reaches over, which would take the press instead. */
  steals: string[];
  /** Room above and below the control inside the bar. Equal means it sits on the bar's centre. */
  seat: { above: number; below: number } | null;
  /** Whether pressing the far corner of the area - outside the paint - opened the drawer. */
  cornerOpensDrawer: boolean | null;
}

let panel: Panel;
const measured = new Map<string, Measured>();
const targets = new Map<string, Target>();

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
    ['the queue', `/root/queue`],
    ['recent queue outcomes', `/root/queue/recent`],
    /* Armed, so the row of actions this file exists for is on the page. An ended
       request draws no buttons and would measure the easy half of the view. */
    ['a queue request', `/root/queue/request/pending-ci-0`],
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

  /* Thirty-eight pages, and each one is a navigation followed by a wait. Swept in lanes because
     the wait is nearly all of it and nothing measured below has a clock in it: a layout viewport
     is the width the content asked for, and a page under load asks for the same width it would
     have asked for alone. */
  const pages = WIDTHS.flatMap((width) =>
    routes(panel.account).map(([name, path]) => ({ key: `${name} at ${width}`, path, width })),
  );
  for (const { key, reading } of await inLanes(pages, async ({ key, path, width }) => ({
    key,
    reading: await measure(path, width),
  }))) {
    measured.set(key, reading);
  }

  for (const { name, target } of await inLanes(BAR_CONTROLS, async ([name, selector]) => ({
    name,
    target: await measureTarget(selector, name === 'the menu button'),
  }))) {
    targets.set(name, target);
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
    await visit(page, `${panel.origin}${path}`);

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

      /* Filtering that went away rather than being crushed. The sweep above
         cannot see this: it skips anything failing `checkVisibility`, and a
         `thead` hidden with `display: none` takes its funnels out of the page
         entirely - no box to measure, no tab stop to find. The queue shipped
         that way, offering a search field and nothing else on a phone.

         So the rule is stated as a relation instead of a measurement: where a
         column heading's filter is on the page but hidden, something else on
         the page has to filter. `TableToolsMenu` is what answers it. */
      const hiddenFilters = [...document.querySelectorAll('.filter-trigger')].filter(
        (control) => !control.checkVisibility(),
      ).length;
      const reachableFilters = [
        ...document.querySelectorAll('.filter-trigger, .tools-trigger'),
      ].filter((control) => control.checkVisibility()).length;

      return {
        layoutViewport: window.innerWidth,
        heading: document.querySelector('h1, h2')?.textContent?.trim() ?? null,
        widest,
        unreachable: [...new Set(unreachable)],
        hiddenFilters,
        reachableFilters,
      };
    }, width);
  } finally {
    await page.close();
  }
}

/** What the top bar offers a thumb on a phone. */
const BAR_CONTROLS = [
  ['the menu button', '.mobile-navigation-trigger'],
  ['the account menu', '.who'],
] as const;

/** The size the platforms ask for, and what the overlay is built to give. */
const THUMB = 44;

async function measureTarget(selector: string, pressCorner: boolean): Promise<Target> {
  const page = await panel.browser.newPage({
    viewport: { width: 375, height: 812 },
    deviceScaleFactor: 2,
    isMobile: true,
    hasTouch: true,
  });

  try {
    await visit(page, `${panel.origin}/i/${panel.account}/settings`);

    const measurement = await page.evaluate((target: string) => {
      const control = document.querySelector(target);
      if (control === null) throw new Error(`${target} is not on the page`);

      const own = control.getBoundingClientRect();
      /* The overlay's box, read from the resolved insets of the pseudo-element
         that draws it. A negative inset is the expansion. */
      const overlay = getComputedStyle(control, '::after');
      const px = (value: string): number => Number.parseFloat(value) || 0;
      const area = {
        left: own.left + px(overlay.left),
        top: own.top + px(overlay.top),
        right: own.right - px(overlay.right),
        bottom: own.bottom - px(overlay.bottom),
      };

      const CONTROLS =
        'button, a[href], input:not([type=hidden]), select, textarea, [role="button"], [role="tab"], [role="switch"], [role="menuitem"]';
      const steals: string[] = [];
      for (const other of document.querySelectorAll(CONTROLS)) {
        if (other === control || control.contains(other) || other.contains(control)) continue;
        if (!other.checkVisibility()) continue;
        if (other.closest('[popover]:not(:popover-open)') !== null) continue;
        if (other.closest('.navigation-shell, .visually-hidden') !== null) continue;
        const box = other.getBoundingClientRect();
        if (box.width === 0 || box.height === 0) continue;
        if (area.left >= box.right || box.left >= area.right) continue;
        if (area.top >= box.bottom || box.top >= area.bottom) continue;
        steals.push(
          `${other.tagName.toLowerCase()} "${(other.getAttribute('aria-label') ?? other.textContent ?? '').replace(/\s+/gu, ' ').trim().slice(0, 30)}"`,
        );
      }

      const bar = document.querySelector('.brand-row')?.getBoundingClientRect() ?? null;

      return {
        control: { width: own.width, height: own.height },
        seat: bar === null ? null : { above: own.top - bar.top, below: bar.bottom - own.bottom },
        area: {
          width: area.right - area.left,
          height: area.bottom - area.top,
          onScreen: area.left >= 0 && area.right <= window.innerWidth && area.top >= 0,
        },
        corner: { x: area.left + 2, y: area.top + 2 },
        steals,
      };
    }, selector);

    let cornerOpensDrawer: boolean | null = null;
    if (pressCorner) {
      /* Pressed where only the overlay is - a couple of pixels inside the far
         corner, well outside the 28px the reader can see. A real press rather
         than `.click()`, which fires the handler whether or not anything would
         actually have received it. */
      await page.mouse.click(measurement.corner.x, measurement.corner.y);
      await page.waitForTimeout(700);
      cornerOpensDrawer = await page.evaluate(
        () => document.querySelector('.navigation-shell')?.checkVisibility() ?? false,
      );
    }

    return {
      control: measurement.control,
      area: measurement.area,
      seat: measurement.seat,
      steals: [...new Set(measurement.steals)],
      cornerOpensDrawer,
    };
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

  /* Hiding the heading band is how every table here becomes a stack of cards, and
     it is the right move - but the funnels live in that band, and a page that
     hides them without putting them anywhere else has quietly dropped a feature
     on the readers least able to work around it. */
  it.each(cases)('still offers a way to filter %s', (key) => {
    const result = measured.get(key);
    if (result === undefined) throw new Error(`${key} was never measured`);
    if (result.hiddenFilters === 0) return;

    expect(
      result.reachableFilters,
      `${key} hides ${result.hiddenFilters} column filter(s) and offers nothing in their place`,
    ).toBeGreaterThan(0);
  });
});

/**
 * The top bar is the one part of the panel every page is reached through, and on
 * a phone its controls are 28-32px squares because that is the weight the bar
 * wants. They keep it: what grows is an invisible overlay that takes the press,
 * so these are measurements of what a thumb can hit rather than of what is drawn.
 */
describe('the top bar on a phone', () => {
  it.each(BAR_CONTROLS.map(([name]) => name))('gives %s a thumb-sized target', (name) => {
    const target = targets.get(name);
    if (target === undefined) throw new Error(`${name} was never measured`);

    // The precondition: the control is still the small square it is meant to be,
    // so a change that simply made it bigger does not pass as this being fixed.
    expect(target.control.width, `${name} is no longer a compact control`).toBeLessThan(THUMB);

    expect(
      Math.round(Math.min(target.area.width, target.area.height)),
      `${name} offers a ${Math.round(target.area.width)}x${Math.round(target.area.height)} target`,
    ).toBeGreaterThanOrEqual(THUMB);
    expect(target.area.onScreen, `${name}'s target runs off the screen`).toBe(true);
  });

  it.each(BAR_CONTROLS.map(([name]) => name))('keeps %s off its neighbours', (name) => {
    const target = targets.get(name);
    if (target === undefined) throw new Error(`${name} was never measured`);

    /* An expanded target that reaches over another control does not make the bar
       easier to use, it makes the neighbour unpressable. */
    expect(
      target.steals,
      `${name}'s target reaches over:\n  ${target.steals.join('\n  ')}`,
    ).toEqual([]);
  });

  it.each(BAR_CONTROLS.map(([name]) => name))('keeps %s seated on the bar', (name) => {
    const target = targets.get(name);
    if (target === undefined) throw new Error(`${name} was never measured`);
    if (target.seat === null) throw new Error('the bar was not found');

    /* Equal room above and below, which is the cheapest true statement about a
       control that is placed rather than one that has fallen into the flow.
       Growing a hit area must not move anything, and the first attempt at it
       did: a `position: relative` added for the overlay outranked the
       `absolute` that puts the menu button in its corner, and the button
       dropped into the row. Every size and overlap check here still passed. */
    expect(
      Math.round(target.seat.above),
      `${name} sits ${target.seat.above.toFixed(1)}px below the top of the bar and ` +
        `${target.seat.below.toFixed(1)}px above the bottom`,
    ).toBe(Math.round(target.seat.below));
  });

  it('opens the drawer from outside the menu button’s paint', () => {
    const target = targets.get('the menu button');
    if (target === undefined) throw new Error('the menu button was never measured');

    /* The whole claim, tested where it is made: a point no part of the visible
       button covers still opens the navigation. Without this the size checks
       above would pass on an overlay that had `pointer-events: none`. */
    expect(target.cornerOpensDrawer).toBe(true);
  });
});

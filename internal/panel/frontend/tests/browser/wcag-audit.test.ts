import type { Page } from 'playwright-core';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { addressOf, inLanes, PANEL_ROUTES, startPanel, visit, type Panel } from './harness';

/**
 * The panel against WCAG 2.2, on every route it has.
 *
 * Only the criteria a machine can settle, and each one asked the way its own Understanding
 * document frames it rather than the way a linter would: the contrast check knows what
 * large text is, the target check knows the spacing exception, and the name check knows
 * that a control whose name is its text needs nothing else.
 */

interface Finding {
  route: string;
  rule: string;
  where: string;
  detail: string;
}

let panel: Panel;
const findings: Finding[] = [];

beforeAll(async () => {
  panel = await startPanel();
  const swept = await inLanes(PANEL_ROUTES, async (route) => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
    try {
      await visit(page, addressOf(panel, route));
      const still = await audit(page);
      const spaced = await spacingAudit(page);
      return [...still, ...spaced].map((one) => ({ ...one, route }));
    } finally {
      await page.close();
    }
  });
  findings.push(...swept.flat());
}, 900_000);

afterAll(async () => {
  await panel?.close();
});

function audit(page: Page): Promise<Omit<Finding, 'route'>[]> {
  return page.evaluate(() => {
    const found: { rule: string; where: string; detail: string }[] = [];

    const name = (element: Element): string => {
      const classes = element.className;
      const list = typeof classes === 'string' ? classes : '';
      const own = list
        .split(/\s+/u)
        .filter((one) => one !== '' && !one.startsWith('svelte-'))
        .slice(0, 2)
        .join('.');
      return element.tagName.toLowerCase() + (own === '' ? '' : `.${own}`);
    };

    const shown = (element: Element): boolean => {
      if (!element.checkVisibility()) return false;
      const box = element.getBoundingClientRect();
      return box.width > 0 && box.height > 0;
    };

    /* THE BROWSER CONVERTS THE COLOUR, because a computed colour is not always sRGB.
       `getComputedStyle` hands back `oklab(0.999994 …)` wherever a token went through a
       wide-gamut mix, and reading those three numbers as if they were channels turns
       white into near-black - which is how this sweep first reported a 2.94:1 on a note
       that measures 6.87:1. Painted onto a 1x1 canvas, every space arrives as bytes. */
    const canvas = document.createElement('canvas');
    canvas.width = 1;
    canvas.height = 1;
    const pen = canvas.getContext('2d', { willReadFrequently: true })!;
    const channels = (colour: string): [number, number, number] => {
      pen.clearRect(0, 0, 1, 1);
      pen.fillStyle = '#000';
      pen.fillStyle = colour;
      pen.fillRect(0, 0, 1, 1);
      const [r, g, b] = pen.getImageData(0, 0, 1, 1).data;
      return [r ?? 0, g ?? 0, b ?? 0];
    };

    const luminance = (colour: string): number => {
      const [r, g, b] = channels(colour).map((value) => {
        const v = value / 255;
        return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
      });
      return 0.2126 * (r ?? 0) + 0.7152 * (g ?? 0) + 0.0722 * (b ?? 0);
    };

    const ratio = (a: string, b: string): number => {
      const [high, low] = [luminance(a), luminance(b)].sort((x, y) => y - x);
      return (high! + 0.05) / (low! + 0.05);
    };

    /* THE GROUND IS WHAT IS PAINTED BEHIND THE WORDS, which is not always an ancestor.
       A selected tree row's fill is the nav thumb and a selected segment's is the
       selection indicator - both siblings, positioned underneath - so walking the
       parents finds the chrome behind them and reports white ink at 1.19:1 on a light
       sidebar that the reader never sees. Asked of the paint stack at the text's own
       midpoint instead, which is the question. */
    const solid = (colour: string): boolean => {
      const parts = (colour.match(/[\d.]+/gu) ?? []).map(Number);
      return colour !== '' && parts.length >= 3 && (parts[3] ?? 1) >= 0.99;
    };

    const opaque = (element: Element): string => {
      const box = element.getBoundingClientRect();
      const x = box.left + box.width / 2;
      const y = box.top + box.height / 2;

      /* A hit test cannot answer this: the two elements that paint a ground for somebody
         else - the nav thumb under a selected row, the indicator under a selected
         segment - are both `pointer-events: none`, so they are absent from the stack
         `elementsFromPoint` returns. Walk the ancestors instead, and at each one look at
         the siblings painted BEFORE it: a positioned box with a fill of its own, covering
         the words, is the ground the reader sees. Without this the sweep reported white
         ink at 1.19:1 in a light sidebar - reading the chrome behind a teal thumb. */
      let node: Element | null = element;
      while (node !== null) {
        /* The node's OWN fill first, and a sibling's only where it has none. The other
           order reads the page a sticky bar floats over rather than the bar itself. */
        const own = getComputedStyle(node).backgroundColor;
        if (solid(own)) return own;
        for (let prior = node.previousElementSibling; prior !== null;) {
          const style = getComputedStyle(prior);
          const theirs = prior.getBoundingClientRect();
          const covers =
            x >= theirs.left && x <= theirs.right && y >= theirs.top && y <= theirs.bottom;
          if (style.position !== 'static' && covers && solid(style.backgroundColor)) {
            return style.backgroundColor;
          }
          prior = prior.previousElementSibling;
        }
        node = node.parentElement;
      }
      return 'rgb(255, 255, 255)';
    };

    /* An accessible name, computed the way a browser computes one, minus the parts a
       page cannot reach from script: content, aria-label, aria-labelledby, title, and -
       for an input - its label. */
    const accessibleName = (element: Element): string => {
      const labelled = element.getAttribute('aria-labelledby');
      if (labelled !== null) {
        const text = labelled
          .split(/\s+/u)
          .map((id) => document.getElementById(id)?.textContent ?? '')
          .join(' ')
          .trim();
        if (text !== '') return text;
      }
      const label = element.getAttribute('aria-label')?.trim();
      if (label !== undefined && label !== '') return label;
      /* A textarea takes a label exactly as an input does, and leaving it out of this
         list reported a labelled field as nameless. */
      if (
        element instanceof HTMLInputElement ||
        element instanceof HTMLSelectElement ||
        element instanceof HTMLTextAreaElement
      ) {
        const own = element.labels?.[0]?.textContent?.trim();
        if (own !== undefined && own !== '') return own;
      }
      const text = (element.textContent ?? '').trim();
      if (text !== '') return text;
      const title = element.getAttribute('title')?.trim();
      return title ?? '';
    };

    const interactive = [
      ...document.querySelectorAll<HTMLElement>(
        'a[href], button, input:not([type="hidden"]), select, textarea, summary, [tabindex]:not([tabindex="-1"]), [role="button"], [role="link"], [role="tab"], [role="switch"], [role="checkbox"]',
      ),
    ].filter((element) => shown(element));

    /* SC 4.1.2 Name, Role, Value (A) - every control has an accessible name. */
    for (const control of interactive) {
      if (accessibleName(control) === '') {
        found.push({
          rule: '4.1.2 Name, Role, Value',
          where: name(control),
          detail: 'no accessible name',
        });
      }
    }

    /* SC 2.5.3 Label in Name (A) - the words a control SHOWS are in the name it answers
       to, so somebody speaking to it can say what they can see.

       The visible label is not simply the text content. A keyboard hint in a `kbd` is a
       shortcut rather than a name; anything visually hidden is already only a name; and
       a monogram in an avatar is a graphic, which the criterion covers under 1.1.1 and
       not here. Each of those is discounted before the comparison, or the check reports
       "Search ⌘K" against "Search this workspace" and buries the real ones. */
    for (const control of interactive) {
      const label = control.getAttribute('aria-label');
      if (label === null) continue;
      const copy = control.cloneNode(true) as HTMLElement;
      for (const spare of copy.querySelectorAll('kbd, .visually-hidden, .avatar, .ws-mini')) {
        spare.remove();
      }
      const visible = (copy.textContent ?? '').replaceAll(/\s+/gu, ' ').trim();
      if (visible === '' || visible.length > 60) continue;
      /* A monogram: two or three capitals standing in for a name the label spells out. */
      if (/^[A-Z]{1,3}$/u.test(visible)) continue;
      /* And a bare number is a VALUE rather than a label. The criterion is about "text
         that labels a component"; a tile showing how many files changed is showing its
         own reading, and nobody speaks to it by saying "six". */
      if (/^[\d\s+~\-−]+$/u.test(visible)) continue;
      const flat = (value: string): string =>
        value
          .toLowerCase()
          .replaceAll(/[^a-z0-9]+/gu, ' ')
          .trim();
      if (!flat(label).includes(flat(visible))) {
        found.push({
          rule: '2.5.3 Label in Name',
          where: name(control),
          detail: `shows "${visible.slice(0, 30)}" but is named "${label.slice(0, 40)}"`,
        });
      }
    }

    /* SC 2.5.8 Target Size (Minimum) (AA) - 24x24 CSS px, unless the target is spaced so
       that a 24px circle centred on it overlaps no other target, or is inline in a
       sentence, or is the user agent's own. */
    /* THE TARGET IS WHAT A FINGER MAY LAND ON, which is neither the element the state
       lives on nor the box it was laid out in. A native checkbox is 13px inside a label
       that is the whole hit area, and a switch grows its target with a pseudo-element
       that has no box of its own to measure - so the question is asked of the page:
       press-test outward from the centre and find where the control stops answering. */
    const answersAt = (element: Element, x: number, y: number): boolean => {
      const hit = document.elementFromPoint(x, y);
      if (hit === null) return false;
      if (hit === element || element.contains(hit)) return true;
      const label = hit.closest('label');
      return label !== null && (label === element || label.contains(element));
    };

    const boxes = interactive.map((element) => {
      const wrapper =
        element instanceof HTMLInputElement && element.labels?.[0] !== undefined
          ? element.labels[0]
          : element;
      const laid = wrapper.getBoundingClientRect();
      /* Laid out big enough already: no need to press the page to find out. */
      if (laid.width >= 24 && laid.height >= 24) {
        return { element, box: laid, target: { width: laid.width, height: laid.height } };
      }
      /* AND IT HAS TO BE ON SCREEN TO BE PRESSED AT. `elementFromPoint` answers null for
         a coordinate outside the viewport, so a control below the fold measures as
         reaching nowhere - which is how a switch whose target is 44px tall reported 20.
         Scrolling each one into view was worse than the disease: the reflow moved every
         rect measured after it, and inputs that are 96x30 in a browser came back 527x10.
         So a control that cannot be pressed at from here is reported as UNMEASURED
         rather than guessed at, and the sweep says how many it could not reach. */
      const reachable =
        laid.top >= 22 &&
        laid.left >= 22 &&
        laid.bottom <= window.innerHeight - 22 &&
        laid.right <= window.innerWidth - 22;
      if (!reachable) {
        return { element, box: laid, target: null };
      }
      const cx = laid.left + laid.width / 2;
      const cy = laid.top + laid.height / 2;
      /* THE QUESTION THE CRITERION ASKS is whether a 24x24 area centred on the target
         belongs to it - so it is asked directly, at the four points 12px out, rather
         than by walking outward a pixel at a time. Walking broke on a single sub-pixel
         miss: one switch answered at 6px and not at 1px, and the scan called its whole
         reach zero. */
      const half = 12;
      const height =
        answersAt(element, cx, cy - half) && answersAt(element, cx, cy + half)
          ? Math.max(laid.height, 24)
          : laid.height;
      const width =
        answersAt(element, cx - half, cy) && answersAt(element, cx + half, cy)
          ? Math.max(laid.width, 24)
          : laid.width;
      return { element, box: laid, target: { width, height } };
    });
    for (const { element, box, target } of boxes) {
      if (target === null) continue;
      if (target.width >= 24 && target.height >= 24) continue;
      /* THE INLINE EXCEPTION, as the criterion words it: "the target is in a sentence, or
         its size is otherwise constrained by the line-height of non-target text". So it
         is not a rule about links - a relative time set inside a row's meta line is the
         same case, and reading it as one cost three false findings. */
      const parentText = (element.parentElement?.textContent ?? '').trim();
      const ownText = (element.textContent ?? '').trim();
      const inline = getComputedStyle(element).display.startsWith('inline');
      if (inline && parentText.length > ownText.length + 12) continue;
      /* The second half of the same exception, which is the half these need: a box
         shorter than the line it is set on is a box the line-height decided, not the
         author. A queue row's PR name is 10px tall inside a 23px line. */
      const line = Number.parseFloat(getComputedStyle(element.parentElement ?? element).lineHeight);
      if (inline && Number.isFinite(line) && box.height <= line) continue;
      const cx = box.left + box.width / 2;
      const cy = box.top + box.height / 2;
      const crowded = boxes.some(({ element: other, box: theirs }) => {
        if (other === element) return false;
        const nx = Math.max(theirs.left, Math.min(cx, theirs.right));
        const ny = Math.max(theirs.top, Math.min(cy, theirs.bottom));
        return Math.hypot(cx - nx, cy - ny) < 12;
      });
      if (!crowded) continue;
      found.push({
        rule: '2.5.8 Target Size (Minimum)',
        where: name(element),
        detail: `target ${target.width.toFixed(0)}x${target.height.toFixed(0)}px (box ${box.width.toFixed(0)}x${box.height.toFixed(0)}) with a neighbour inside 24px`,
      });
    }

    /* SC 1.4.3 Contrast (Minimum) (AA) - 4.5:1, or 3:1 for large text (18.66px bold, or
       24px). Read on the innermost element that holds the words. */
    for (const element of document.querySelectorAll<HTMLElement>('*')) {
      if (!shown(element)) continue;
      const text = (element.textContent ?? '').trim();
      if (text === '') continue;
      if ([...element.children].some((child) => (child.textContent ?? '').trim() !== '')) continue;
      const style = getComputedStyle(element);
      if (style.opacity !== '' && Number.parseFloat(style.opacity) < 0.99) continue;
      const size = Number.parseFloat(style.fontSize);
      const bold = Number.parseInt(style.fontWeight, 10) >= 700;
      const large = size >= 24 || (bold && size >= 18.66);
      const owed = large ? 3 : 4.5;
      const ground = opaque(element);
      const measured = ratio(style.color, ground);
      if (measured + 0.05 < owed) {
        found.push({
          rule: '1.4.3 Contrast (Minimum)',
          where: name(element),
          detail: `${measured.toFixed(2)}:1 on ${size.toFixed(0)}px${bold ? ' bold' : ''}, owed ${owed}:1 — "${text.slice(0, 26)}"`,
        });
      }
    }

    /* SC 1.3.1 Info and Relationships (A) - a heading level is never skipped, because the
       levels are the document's outline and a gap is a level a reader is told exists. */
    const levels = [...document.querySelectorAll<HTMLElement>('h1, h2, h3, h4, h5, h6')]
      .filter((heading) => shown(heading))
      .map((heading) => ({ heading, level: Number.parseInt(heading.tagName.slice(1), 10) }));
    let previous = 0;
    for (const { heading, level } of levels) {
      if (previous !== 0 && level > previous + 1) {
        found.push({
          rule: '1.3.1 Info and Relationships',
          where: heading.tagName.toLowerCase(),
          detail: `h${previous} is followed by h${level} — "${(heading.textContent ?? '').trim().slice(0, 30)}"`,
        });
      }
      previous = level;
    }

    /* SC 2.4.7 Focus Visible (A) - every control that takes focus shows that it has it.
       Asked of the STYLESHEET rather than by focusing each one: `:focus-visible` matches
       on a real keyboard journey and not reliably on a scripted `focus()`, so a probe
       that focused everything would report the indicator missing wherever the browser
       decided the focus was not keyboard-driven. What can be settled exactly is whether
       a rule exists that reaches this element and paints something. */
    const focusPainters = new Set<string>();
    for (const sheet of document.styleSheets) {
      let rules: CSSRuleList;
      try {
        rules = sheet.cssRules;
      } catch {
        continue;
      }
      const walk = (list: CSSRuleList): void => {
        for (const rule of list) {
          if (rule instanceof CSSGroupingRule) walk(rule.cssRules);
          if (!(rule instanceof CSSStyleRule)) continue;
          if (!rule.selectorText.includes(':focus')) continue;
          const paints = ['outline', 'outline-color', 'box-shadow', 'border-color', 'background']
            .map((property) => rule.style.getPropertyValue(property))
            .some((value) => value !== '' && value !== 'none' && value !== 'transparent');
          if (!paints) continue;
          for (const one of rule.selectorText.split(/,(?![^(]*\))/u)) {
            focusPainters.add(one.replaceAll(/:focus(-visible|-within)?/gu, '').trim());
          }
        }
      };
      walk(rules);
    }
    for (const control of interactive) {
      const lit = [...focusPainters].some((selector) => {
        /* A bare `:focus-visible` strips to nothing, and nothing is what the whole
           document matches - the panel has exactly one such rule and it is the reason
           most of these controls have a ring at all. Read as "matches nobody", it
           reported 991 controls with no focus style and buried the question. */
        if (selector === '') return true;
        try {
          return control.matches(selector);
        } catch {
          return false;
        }
      });
      if (!lit) {
        found.push({
          rule: '2.4.7 Focus Visible',
          where: name(control),
          detail: 'no :focus rule reaches it that paints anything',
        });
      }
    }

    /* SC 2.4.11 Focus Not Obscured (Minimum) (AA) - when a control takes focus it is not
       ENTIRELY hidden by author-created content. The criterion is about the thing that
       floats: a sticky bar, a docked toolbar, a fixed footer. So the question is asked of
       those and not of the paint stack in general - read the other way it reported every
       transparent input behind its own switch track, which is not content on top but the
       control's own construction. */
    const floating = [...document.querySelectorAll<HTMLElement>('*')].filter((element) => {
      const style = getComputedStyle(element);
      return (
        (style.position === 'sticky' || style.position === 'fixed') &&
        element.checkVisibility() &&
        solid(style.backgroundColor)
      );
    });
    for (const control of interactive) {
      const box = control.getBoundingClientRect();
      if (box.bottom < 0 || box.top > window.innerHeight || box.width === 0) continue;
      const buried = floating.some((cover) => {
        if (cover.contains(control) || control.contains(cover)) return false;
        const over = cover.getBoundingClientRect();
        return (
          over.left <= box.left &&
          over.right >= box.right &&
          over.top <= box.top &&
          over.bottom >= box.bottom
        );
      });
      if (buried) {
        found.push({
          rule: '2.4.11 Focus Not Obscured',
          where: name(control),
          detail: 'wholly covered by a sticky or fixed element',
        });
      }
    }

    /* SC 2.4.2 Page Titled (A) and 3.1.1 Language of Page (A). */
    if (document.title.trim() === '') {
      found.push({ rule: '2.4.2 Page Titled', where: 'document', detail: 'no title' });
    }
    if ((document.documentElement.lang ?? '').trim() === '') {
      found.push({ rule: '3.1.1 Language of Page', where: 'html', detail: 'no lang attribute' });
    }

    /* SC 2.4.1 Bypass Blocks (A) - every one of these pages puts a rail and a sidebar
       before its content, and a keyboard reader meets both on every route. */
    const skip = [...document.querySelectorAll('a[href^="#"]')].some((link) =>
      /skip|jump/iu.test(link.textContent ?? ''),
    );
    const landmarks = document.querySelectorAll('main, [role="main"]').length;
    if (!skip && landmarks === 0) {
      found.push({
        rule: '2.4.1 Bypass Blocks',
        where: 'document',
        detail: 'no skip link and no main landmark',
      });
    }

    /* SC 3.3.2 Labels or Instructions (A) - every field that takes input has a label. */
    for (const field of document.querySelectorAll<HTMLInputElement | HTMLTextAreaElement>(
      'input:not([type="hidden"]):not([type="checkbox"]):not([type="radio"]), textarea, select',
    )) {
      if (!shown(field)) continue;
      if (accessibleName(field) === '' && (field.getAttribute('placeholder') ?? '') === '') {
        found.push({
          rule: '3.3.2 Labels or Instructions',
          where: name(field),
          detail: 'field has neither a label nor a placeholder',
        });
      }
    }

    return found;
  });
}

/**
 * SC 1.4.12 Text Spacing (AA), applied exactly as the criterion words it.
 *
 * A reader may set line height to 1.5x the font size, space after a paragraph to 2x,
 * letter spacing to 0.12em and word spacing to 0.16em, and no content or function may be
 * lost. It is a real test rather than a review: the four declarations go on, and anything
 * whose words are then cut off by a box that clips is content the reader lost.
 *
 * A page that scrolls is not a failure - growing taller is what is supposed to happen.
 * What fails is a box that keeps its size and hides the difference, which is why this
 * asks only about elements that clip on an axis their content has outgrown.
 */
function spacingAudit(page: Page): Promise<Omit<Finding, 'route'>[]> {
  return page.evaluate(() => {
    const sheet = document.createElement('style');
    sheet.textContent = `* {
      line-height: 1.5 !important;
      letter-spacing: 0.12em !important;
      word-spacing: 0.16em !important;
    }
    p { margin-block-end: 2em !important; }`;
    document.head.append(sheet);
    /* Forced, so the measurements below are taken after the reflow rather than during. */
    void document.body.getBoundingClientRect();

    const found: { rule: string; where: string; detail: string }[] = [];
    const named = (element: Element): string => {
      const classes = element.className;
      const list = typeof classes === 'string' ? classes : '';
      const own = list
        .split(/\s+/u)
        .filter((one) => one !== '' && !one.startsWith('svelte-'))
        .slice(0, 2)
        .join('.');
      return element.tagName.toLowerCase() + (own === '' ? '' : `.${own}`);
    };

    for (const element of document.querySelectorAll<HTMLElement>('*')) {
      if (!element.checkVisibility()) continue;
      const text = (element.textContent ?? '').trim();
      if (text === '') continue;
      /* A box that clips ON PURPOSE and is not showing anything: the visually-hidden
         recipe is a 1px clipped box, and every one of them "loses" its words to a
         reader who was never going to see them. */
      if (element.closest('.visually-hidden') !== null) continue;
      const style = getComputedStyle(element);
      if (Number.parseFloat(style.opacity) < 0.99) continue;
      /* The same recipe written without the class - a `<legend>` naming a fieldset is
         4x1px under `clip-path: inset(50%)` - is the same thing and not a loss. Read as
         one, it filed 105 findings against words nobody was ever shown. */
      const box = element.getBoundingClientRect();
      if (box.width <= 8 || box.height <= 8) continue;
      if (style.clipPath.startsWith('inset(50%')) continue;
      /* And a box whose overflow is hidden but which HOLDS a scroller has lost nothing:
         the reader reaches the rest by scrolling, which is what the criterion allows a
         page to do. Only a clip with no way past it is a loss. */
      const scrollable = [...element.querySelectorAll<HTMLElement>('*')].some((inner) => {
        const own = getComputedStyle(inner);
        return (
          own.overflowY === 'auto' ||
          own.overflowY === 'scroll' ||
          own.overflowX === 'auto' ||
          own.overflowX === 'scroll'
        );
      });
      if (scrollable) continue;
      const clipsY = style.overflowY === 'hidden' || style.overflowY === 'clip';
      const clipsX = style.overflowX === 'hidden' || style.overflowX === 'clip';
      /* A single line that says so is not a loss: `text-overflow: ellipsis` is a
         deliberate truncation with the whole string still in the accessible name, and
         the criterion's own understanding treats that as content still available. */
      if (style.textOverflow === 'ellipsis') continue;
      const lostY = clipsY && element.scrollHeight > element.clientHeight + 1;
      const lostX = clipsX && element.scrollWidth > element.clientWidth + 1;
      if (!lostY && !lostX) continue;
      found.push({
        rule: '1.4.12 Text Spacing',
        where: named(element),
        detail: `${lostY ? `${element.scrollHeight - element.clientHeight}px of height` : `${element.scrollWidth - element.clientWidth}px of width`} is cut off — "${text.slice(0, 26)}"`,
      });
    }

    sheet.remove();
    return found;
  });
}

/**
 * NOTHING IS OPEN. Every criterion this sweep can settle, the panel settles.
 *
 * It held six for a long time, across three components, and none of them needed the
 * design decision they were waiting on.
 *
 * Two were never really failing. A bare switch already grows its press area to 44px
 * with a pseudo-element, and this sweep asks the page directly - it presses 12px out
 * from the centre and sees who answers. What it cannot do is press at a coordinate
 * outside the viewport, where `elementFromPoint` returns null. Four list views used to
 * pin the pane to the viewport and scroll their rows inside it, which put row after row
 * past the bottom edge with the probe unable to reach any of them, so every control on
 * them measured at its bare box. Those pages scroll as documents now, the probe reaches
 * the rows, and the findings went with them.
 *
 * The third was real: a queue row's pull-request link, 203x10 of ink. The note that
 * used to stand here said 24px meant taller rows and that this was a decision rather
 * than a fix - and it was neither. The link's own box grew to 24 while its words did
 * not move (`align-content` centres the one flex line it holds), and the row did not
 * grow at all, because the reason beside it already ran to two lines and was taller
 * than both.
 *
 * Keep this list empty. An entry here is a criterion the panel has decided not to
 * meet, and there is no longer one of those.
 */
const OPEN: ReadonlyArray<{ where: string; hits: number }> = [];

describe('WCAG 2.2, on every route [Integration]', () => {
  it('finds nothing a machine can settle beyond what is knowingly open', () => {
    /* The knowingly-open six are held to their exact count: one more of them, or one on a
       component not listed, is a new fault and fails here. */
    const allowance = new Map(OPEN.map((one) => [one.where, one.hits]));
    const fresh = findings.filter((one) => {
      const left = allowance.get(one.where);
      if (left === undefined || left === 0) return true;
      allowance.set(one.where, left - 1);
      return false;
    });
    const unspent = [...allowance.entries()].filter(([, left]) => left > 0);

    const byRule = new Map<string, Finding[]>();
    for (const one of fresh) {
      byRule.set(one.rule, [...(byRule.get(one.rule) ?? []), one]);
    }
    const report = [...byRule.entries()]
      .sort((a, b) => b[1].length - a[1].length)
      .map(([rule, list]) => {
        /* Grouped by WHERE, not by route: one component drawn on twenty routes is one
           thing to fix, and a list of twenty routes says the opposite. */
        const byWhere = new Map<string, Finding[]>();
        for (const one of list) {
          byWhere.set(one.where, [...(byWhere.get(one.where) ?? []), one]);
        }
        const lines = [...byWhere.entries()]
          .sort((a, b) => b[1].length - a[1].length)
          .map(
            ([where, hits]) =>
              `      ${where} ×${hits.length} on ${new Set(hits.map((hit) => hit.route)).size} routes — ${hits[0]!.detail}`,
          );
        return [`  ${rule} — ${list.length}`, ...lines].join('\n');
      });
    expect(fresh, `WCAG findings:\n${report.join('\n')}`).toEqual([]);
    /* And the list shrinks only by being fixed: an allowance nobody spent means the
       component was mended and the entry should go, which this says out loud rather
       than letting the list rot. */
    expect(
      unspent,
      `these are listed as open and no longer occur - take them off OPEN: ${unspent
        .map(([where, left]) => `${where} (${left})`)
        .join(', ')}`,
    ).toEqual([]);
  });
});

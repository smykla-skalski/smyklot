import type { Locator } from 'playwright-core';

/** Compare visible SVG ink with the first text line's cap-to-baseline band. */
export function calloutGeometry(callouts: Locator) {
  return callouts.evaluateAll((elements) => {
    function capHeight(style: CSSStyleDeclaration) {
      const probe = document.createElement('span');
      probe.textContent = 'H';
      probe.style.cssText = `position:absolute;display:inline-block;text-box:trim-both cap alphabetic;line-height:1;font-family:${style.fontFamily};font-size:${style.fontSize};font-weight:${style.fontWeight}`;
      document.body.append(probe);
      const height = probe.getBoundingClientRect().height;
      probe.remove();
      return height;
    }

    function wrappedGaps(element: Element) {
      const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
      const gaps: number[] = [];
      while (walker.nextNode() !== null) {
        const run = walker.currentNode as Text;
        if (
          !run.textContent?.trim() ||
          !run.parentElement ||
          run.parentElement.closest('svg,button,.callout-actions,.attention-actions')
        )
          continue;
        const range = document.createRange();
        range.selectNodeContents(run);
        const lines = Array.from(range.getClientRects()).filter(
          (rect) => rect.width > 0.5 && rect.height > 0.5,
        );
        const cap = capHeight(getComputedStyle(run.parentElement));
        for (let index = 1; index < lines.length; index++) {
          gaps.push(lines[index]!.top - lines[index - 1]!.top - cap);
        }
      }
      return gaps;
    }

    function textBand(element: Element, edge: 'first' | 'last') {
      const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
      const runs: Text[] = [];
      while (walker.nextNode() !== null) {
        const candidate = walker.currentNode as Text;
        if (
          candidate.textContent?.trim() &&
          !candidate.parentElement?.closest('svg,button,.callout-actions,.attention-actions')
        )
          runs.push(candidate);
      }
      const run = edge === 'first' ? runs[0] : runs.at(-1);
      if (!run?.parentElement) throw new Error('Callout has no visible text');
      const wrapper = document.createElement('span');
      run.parentNode!.insertBefore(wrapper, run);
      wrapper.append(run);
      const strut = document.createElement('span');
      strut.style.cssText = 'display:inline-block;width:0;height:0;vertical-align:baseline';
      if (edge === 'first') wrapper.prepend(strut);
      else wrapper.append(strut);
      const baseline = strut.getBoundingClientRect().top;
      const style = getComputedStyle(wrapper);
      const cap = capHeight(style);
      strut.remove();
      wrapper.replaceWith(run);
      return { top: baseline - cap, baseline, center: baseline - cap / 2 };
    }

    return elements.map((callout) => {
      const firstLine = textBand(callout, 'first');
      const lastLine = textBand(callout, 'last');
      const box = callout.getBoundingClientRect();
      const style = getComputedStyle(callout);
      const svg = callout.querySelector('svg')!;
      const bounds = svg.getBBox();
      const point = new DOMPoint(
        bounds.x + bounds.width / 2,
        bounds.y + bounds.height / 2,
      ).matrixTransform(svg.getScreenCTM()!);
      const copy = callout.querySelector('.callout-copy');
      const heading = copy?.firstElementChild;
      const detail = copy?.lastElementChild;
      return {
        lineGaps: wrappedGaps(callout),
        topSpace: firstLine.top - box.top - parseFloat(style.borderTopWidth),
        bottomSpace: box.bottom - lastLine.baseline - parseFloat(style.borderBottomWidth),
        copyGap:
          heading && detail && heading !== detail
            ? textBand(detail, 'first').top - textBand(heading, 'last').baseline
            : null,
        name: callout.getAttribute('data-case') ?? callout.textContent?.trim(),
        difference: point.y - firstLine.center,
        overflow: callout.scrollWidth - callout.clientWidth,
      };
    });
  });
}

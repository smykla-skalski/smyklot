// @vitest-environment jsdom
import { fireEvent, render, waitFor } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';

import SectionTabs from '../src/lib/components/SectionTabs.svelte';

const items = ['Overview', 'Labels', 'Settings', 'Rulesets', 'Files', 'Plan'].map(
  (label, index) => ({ id: label.toLowerCase(), label, href: `#${index}` }),
);

function rect(left: number, width: number): DOMRect {
  return { left, right: left + width, top: 0, bottom: 40, width, height: 40 } as DOMRect;
}

describe('SectionTabs [Component]', () => {
  afterEach(() => {
    Reflect.deleteProperty(document, 'fonts');
    vi.restoreAllMocks();
  });

  it('does not reveal the active tab again when another tab is hovered', async () => {
    let releaseFonts!: () => void;
    const fontsReady = new Promise<void>((resolve) => (releaseFonts = resolve));
    Object.defineProperty(document, 'fonts', {
      configurable: true,
      value: { ready: fontsReady },
    });
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (
      this: HTMLElement,
    ) {
      if (this.classList.contains('section-tabs')) return rect(0, 320);
      const link = this.closest('a');
      if (link === null) return rect(0, 0);
      const nav = link.closest<HTMLElement>('.section-tabs');
      const label = link.querySelector('.tab-word')?.textContent?.trim() ?? '';
      const index = items.findIndex((item) => item.label === label);
      const left = index * 100 - (nav?.scrollLeft ?? 0);
      return this === link ? rect(left, 100) : rect(left + 10, 70);
    });

    const { container } = render(SectionTabs, {
      items,
      active: 'overview',
      label: 'Sync sections',
      onNavigate: () => {},
    });
    const nav = container.querySelector<HTMLElement>('.section-tabs');
    const files = [...container.querySelectorAll<HTMLAnchorElement>('a')].find(
      (link) => link.textContent?.trim() === 'Files',
    );
    expect(nav).not.toBeNull();
    expect(files).not.toBeUndefined();
    await waitFor(() => expect(container.querySelector('.section-tabs-bar')).not.toBeNull());

    (nav as HTMLElement).scrollLeft = 280;
    await fireEvent.mouseEnter(files as HTMLAnchorElement);
    await tick();

    expect((nav as HTMLElement).scrollLeft).toBe(280);

    releaseFonts();
    await fontsReady;
    await tick();

    expect((nav as HTMLElement).scrollLeft).toBe(280);
  });
});

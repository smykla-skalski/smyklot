import type { Locator } from 'playwright-core';

export async function restingButtonWidth(button: Locator, label: string): Promise<number> {
  return button.evaluate((element, text) => {
    const baseline = element.cloneNode(true) as HTMLElement;
    const content = baseline.querySelector('.button-label')!;
    content.className = 'button-label';
    content.textContent = text;
    baseline.style.position = 'fixed';
    baseline.style.visibility = 'hidden';
    element.after(baseline);
    const width = baseline.getBoundingClientRect().width;
    baseline.remove();
    return width;
  }, label);
}

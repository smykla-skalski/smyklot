/** Reveal a focused control with the smallest scroll, clear of the header and save composer. */
export function revealControl(control: HTMLElement): void {
  const bounds = control.getBoundingClientRect();
  if (bounds.width === 0 || bounds.height === 0) return;
  const composer = document.querySelector('.settings-composer')?.getBoundingClientRect();
  const lowerEdge =
    Math.min(
      window.innerHeight,
      composer && composer.top > 0 && composer.top < window.innerHeight && composer.height > 0
        ? composer.top
        : window.innerHeight,
    ) - 8;
  const upperEdge =
    Math.max(0, document.querySelector('.top-bar')?.getBoundingClientRect().bottom ?? 0) + 8;
  const delta =
    bounds.bottom > lowerEdge
      ? bounds.bottom - lowerEdge
      : bounds.top < upperEdge
        ? bounds.top - upperEdge
        : 0;
  if (delta) window.scrollBy({ top: delta, behavior: 'instant' });
}

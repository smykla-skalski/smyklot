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

const composerClearances = new Map<HTMLElement, number>();

/** Keep native scrolling and the end of the page clear of the measured save bar. */
export function reserveComposerSpace(composer: HTMLElement): () => void {
  const root = document.documentElement;
  const publish = () => {
    if (composerClearances.size === 0) root.style.removeProperty('--settings-composer-clearance');
    else
      root.style.setProperty(
        '--settings-composer-clearance',
        `${Math.max(...composerClearances.values())}px`,
      );
  };
  const update = () => {
    const height = composer.offsetHeight;
    const bottom = Number.parseFloat(getComputedStyle(composer).bottom) || 0;
    composerClearances.set(composer, height + bottom + 8);
    publish();
  };
  const observer = new ResizeObserver(update);
  observer.observe(composer);
  update();
  return () => {
    observer.disconnect();
    composerClearances.delete(composer);
    publish();
  };
}

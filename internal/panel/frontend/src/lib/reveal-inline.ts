/** Keep an item inside the visible inline edge of its horizontal scroll container. */
export function revealInline(container: HTMLElement, item: HTMLElement): void {
  const frame = container.getBoundingClientRect();
  const box = item.getBoundingClientRect();

  if (box.left < frame.left) {
    container.scrollLeft -= frame.left - box.left;
  } else if (box.right > frame.right) {
    container.scrollLeft += box.right - frame.right;
  }
}

/** Reveal the current item now and whenever its scroll container changes size. */
export function observeInlineSelection(
  container: HTMLElement,
  selector = "[aria-current='page']",
): () => void {
  const reveal = (): void => {
    const item = container.querySelector<HTMLElement>(selector);
    if (item !== null) revealInline(container, item);
  };
  const observer = new ResizeObserver(reveal);
  observer.observe(container);
  reveal();
  return () => observer.disconnect();
}

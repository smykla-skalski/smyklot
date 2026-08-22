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

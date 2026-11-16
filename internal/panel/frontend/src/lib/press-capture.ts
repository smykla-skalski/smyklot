const PRESSABLE = 'button:not(:disabled), a[href]';

function inside(bounds: DOMRect, event: PointerEvent): boolean {
  return (
    event.clientX >= bounds.left &&
    event.clientX < bounds.right &&
    event.clientY >= bounds.top &&
    event.clientY < bounds.bottom
  );
}

export function keepPressesOnControls(root: Document): () => void {
  function press(event: PointerEvent): void {
    if (event.defaultPrevented || event.button !== 0 || event.pointerType === 'touch') return;
    const control = event.target instanceof Element ? event.target.closest(PRESSABLE) : null;
    if (!(control instanceof HTMLElement)) return;
    const bounds = control.getBoundingClientRect();
    try {
      control.setPointerCapture(event.pointerId);
    } catch {
      return;
    }
    const leave = (move: PointerEvent): void => {
      if (!inside(bounds, move)) control.releasePointerCapture(move.pointerId);
    };
    control.addEventListener('pointermove', leave);
    control.addEventListener(
      'lostpointercapture',
      () => control.removeEventListener('pointermove', leave),
      { once: true },
    );
  }
  root.addEventListener('pointerdown', press);
  return () => root.removeEventListener('pointerdown', press);
}

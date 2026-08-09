export function modalElementIds(id: string): { title: string; description: string } {
  return { title: `${id}-title`, description: `${id}-description` };
}

export function initialModalFocus(container: ParentNode): HTMLElement | null {
  return (
    container.querySelector<HTMLElement>('[data-modal-focus]') ??
    container.querySelector<HTMLElement>('input, textarea, select, button')
  );
}

import { fireEvent, screen } from '@testing-library/svelte';

/** Choose an actual themed option through the same open-menu interaction as a reader. */
export async function chooseOption(control: HTMLElement, label: string): Promise<void> {
  await fireEvent.keyDown(control, { key: 'ArrowDown' });
  const option = await screen.findByRole('option', { name: label });
  await fireEvent.pointerUp(option, { pointerType: 'touch' });
  await fireEvent.click(option);
}

export async function optionLabels(control: HTMLElement): Promise<string[]> {
  await fireEvent.keyDown(control, { key: 'ArrowDown' });
  const options = await screen.findAllByRole('option');
  const labels = options.map((option) => option.textContent?.trim() ?? '');
  await fireEvent.keyDown(control, { key: 'Escape' });
  return labels;
}

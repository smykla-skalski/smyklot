// @vitest-environment jsdom
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import Select from '../src/lib/components/Select.svelte';
import SelectHarness from './support/SelectHarness.svelte';
import { chooseOption } from './support/select';

const options = [
  { value: 1, label: 'One' },
  { value: 2, label: 'Two' },
];
describe('shared themed Select [Component]', () => {
  it('emits typed values through the visible menu', async () => {
    const onValueChange = vi.fn();
    render(Select, { value: 1, options, onValueChange, 'aria-label': 'Count' });
    await chooseOption(screen.getByRole('combobox', { name: 'Count' }), 'Two');
    expect(onValueChange).toHaveBeenLastCalledWith(2);
  });
  it('preserves numeric, string, empty and null choices and resets its form default', async () => {
    render(SelectHarness);
    const picker = screen.getByLabelText('Typed choice');
    for (const [label, result] of [
      ['Text one', 'string:1'],
      ['Empty value', 'string:'],
      ['No override', 'null'],
      ['Numeric one', 'number:1'],
    ]) {
      await chooseOption(picker, label);
      expect(screen.getByLabelText('Typed result').textContent).toBe(result);
    }
    await chooseOption(picker, 'Zulu');
    // The real-browser suite also verifies reset-button activation and FormData.
    await fireEvent.reset(screen.getByRole('button', { name: 'Reset' }).closest('form')!);
    await waitFor(() => expect(screen.getByLabelText('Typed result').textContent).toBe('number:1'));
  });
});

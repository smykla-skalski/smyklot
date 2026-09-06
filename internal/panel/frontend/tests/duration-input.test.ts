// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';

import DurationInput from '../src/lib/components/DurationInput.svelte';
import { exactDurationSeconds, UNIT_SECONDS, type DurationUnit } from '../src/lib/duration';

const input = () =>
  screen.getByRole('textbox', { name: 'Quiet period amount' }) as HTMLInputElement;
const picker = () =>
  screen.getByRole('combobox', { name: 'Quiet period unit' }) as HTMLSelectElement;

describe('DurationInput [Component]', () => {
  it('uses shared input and select controls and changes units without staging a duration', async () => {
    const onChange = vi.fn();
    render(DurationInput, { value: 90, label: 'Quiet period', onChange });
    expect(input().classList.contains('text-input')).toBe(true);
    expect(picker().classList.contains('select-input')).toBe(true);
    await fireEvent.change(picker(), { target: { value: 'minutes' } });
    expect(input().value).toBe('1.5');
    await fireEvent.change(picker(), { target: { value: 'hours' } });
    expect(exactDurationSeconds({ amount: input().value, unit: 'hours' })).toBe(90);
    expect(onChange).not.toHaveBeenCalled();
    await fireEvent.input(input(), { target: { value: '0.5' } });
    expect(onChange).toHaveBeenLastCalledWith(1_800);
  });

  it.each([1, 59, 61, 3_599, 86_399])(
    'preserves all %i seconds across every unit',
    async (seconds) => {
      const onChange = vi.fn();
      render(DurationInput, { value: seconds, label: 'Quiet period', onChange });
      for (const unit of ['hours', 'minutes', 'seconds'] as DurationUnit[]) {
        await fireEvent.change(picker(), { target: { value: unit } });
        expect(exactDurationSeconds({ amount: input().value, unit })).toBe(seconds);
      }
      expect(onChange).not.toHaveBeenCalled();
    },
  );

  it('keeps inheritance blank while converting its placeholder and clearing restores inheritance', async () => {
    const onChange = vi.fn();
    render(DurationInput, {
      value: null,
      inherited: 90,
      allowEmpty: true,
      label: 'Quiet period',
      onChange,
    });
    expect(input().value).toBe('');
    expect(input().placeholder).toBe('90');
    await fireEvent.change(picker(), { target: { value: 'minutes' } });
    expect(input().value).toBe('');
    expect(input().placeholder).toBe('1.5');
    expect(onChange).not.toHaveBeenCalled();
    await fireEvent.input(input(), { target: { value: '2' } });
    expect(onChange).toHaveBeenLastCalledWith(120);
    await fireEvent.input(input(), { target: { value: '' } });
    expect(onChange).toHaveBeenLastCalledWith(null);
  });

  it.each(['', '-1', '0.5', '1e999', 'no', '86401'])(
    'retains invalid %s without changing saved seconds',
    async (value) => {
      const onChange = vi.fn();
      const onValidityChange = vi.fn();
      render(DurationInput, {
        value: 30,
        label: 'Quiet period',
        maximum: 86_400,
        onChange,
        onValidityChange,
      });
      await fireEvent.input(input(), { target: { value } });
      expect(input().value).toBe(value);
      expect(input().getAttribute('aria-invalid')).toBe('true');
      expect(onChange).not.toHaveBeenCalled();
      expect(onValidityChange).toHaveBeenLastCalledWith(expect.any(String));
    },
  );

  it('preserves typing through its own updates and restores externally discarded values', async () => {
    const onChange = vi.fn();
    const props = { value: 30, label: 'Quiet period', onChange };
    const view = render(DurationInput, props);
    await fireEvent.input(input(), { target: { value: '120' } });
    await view.rerender({ ...props, value: 120 });
    expect(input().value).toBe('120');
    expect(picker().value).toBe('seconds');
    await view.rerender(props);
    expect(input().value).toBe('30');
    onChange.mockClear();
    await fireEvent.change(picker(), { target: { value: 'minutes' } });
    expect(input().value).toBe('0.5');
    expect(onChange).not.toHaveBeenCalled();
  });

  it('keeps invalid raw drafts available to their owner and never emits rounded wire values', async () => {
    const onEdit = vi.fn();
    const onChange = vi.fn();
    render(DurationInput, {
      value: 60,
      label: 'Quiet period',
      editor: { amount: '', unit: 'seconds' },
      onEdit,
      onChange,
    });
    await fireEvent.input(input(), { target: { value: '0.5' } });
    expect(onEdit).toHaveBeenCalledWith({ amount: '0.5', unit: 'seconds' });
    expect(onChange).not.toHaveBeenCalled();
  });

  it('resynchronizes validity and the accepted wire value when bounds change', async () => {
    const onChange = vi.fn();
    const onValidityChange = vi.fn();
    const props = {
      value: 30,
      minimum: 1,
      maximum: 86_400,
      label: 'Quiet period',
      onChange,
      onValidityChange,
    };
    const view = render(DurationInput, props);
    await fireEvent.input(input(), { target: { value: '0' } });
    expect(input().getAttribute('aria-invalid')).toBe('true');
    expect(onChange).not.toHaveBeenCalled();
    await view.rerender({ ...props, minimum: 0 });
    expect(input().value).toBe('0');
    expect(input().getAttribute('aria-invalid')).toBe('false');
    expect(onValidityChange).toHaveBeenLastCalledWith(null);
    expect(onChange).toHaveBeenLastCalledWith(0);
    await view.rerender({ ...props, value: 0 });
    expect(input().getAttribute('aria-invalid')).toBe('true');
    expect(onValidityChange).toHaveBeenLastCalledWith(expect.any(String));
  });

  it('clears an externally discarded raw draft even when its seconds never changed', async () => {
    const onEdit = vi.fn();
    const onValidityChange = vi.fn();
    const props = { value: 30, label: 'Quiet period', onEdit, onValidityChange };
    const view = render(DurationInput, props);
    await fireEvent.input(input(), { target: { value: '0.5' } });
    await view.rerender({ ...props, editor: { amount: '0.5', unit: 'seconds' } });
    expect(input().value).toBe('0.5');
    expect(input().getAttribute('aria-invalid')).toBe('true');
    await view.rerender({ ...props, editor: null });
    expect(input().value).toBe('30');
    expect(input().getAttribute('aria-invalid')).toBe('false');
    expect(onValidityChange).toHaveBeenLastCalledWith(null);
  });

  it('does not let a unit change repair invalid text or drop its validation problem', async () => {
    const onChange = vi.fn();
    const onValidityChange = vi.fn();
    render(DurationInput, {
      value: 30,
      label: 'Quiet period',
      maximum: 86_400,
      onChange,
      onValidityChange,
    });
    await fireEvent.input(input(), { target: { value: '0.5' } });
    expect(picker().disabled).toBe(true);
    await fireEvent.change(picker(), { target: { value: 'minutes' } });
    expect(input().value).toBe('0.5');
    expect(input().getAttribute('aria-invalid')).toBe('true');
    expect(onChange).not.toHaveBeenCalled();
    expect(onValidityChange).toHaveBeenLastCalledWith('Enter a duration in whole seconds');
  });

  it('retains a bound violation when a different unit presents the same invalid duration', async () => {
    const onChange = vi.fn();
    const onValidityChange = vi.fn();
    render(DurationInput, {
      value: 30,
      label: 'Quiet period',
      maximum: 86_400,
      onChange,
      onValidityChange,
    });
    await fireEvent.input(input(), { target: { value: '90000' } });
    await fireEvent.change(picker(), { target: { value: 'hours' } });
    expect(input().value).toBe('25');
    expect(input().getAttribute('aria-invalid')).toBe('true');
    expect(onChange).not.toHaveBeenCalled();
    expect(onValidityChange).toHaveBeenLastCalledWith(
      'Quiet period must be from 0 seconds to 1 day',
    );
  });

  it('closes both controls for read-only settings', () => {
    render(DurationInput, { value: 30, label: 'Quiet period', disabled: true });
    expect(input().disabled).toBe(true);
    expect(picker().disabled).toBe(true);
  });
});

describe('exact duration serialization [Unit]', () => {
  it('accepts decimal units only when they represent whole seconds', () => {
    expect(exactDurationSeconds({ amount: '1.5', unit: 'minutes' })).toBe(90);
    expect(exactDurationSeconds({ amount: '0.5', unit: 'seconds' })).toBeNull();
    expect(exactDurationSeconds({ amount: '1e999', unit: 'hours' })).toBeNull();
    for (const seconds of [0, 1, 59, 61, 3599, 86400, 604800]) {
      for (const unit of Object.keys(UNIT_SECONDS) as DurationUnit[]) {
        expect(exactDurationSeconds({ amount: String(seconds / UNIT_SECONDS[unit]), unit })).toBe(
          seconds,
        );
      }
    }
  });
});

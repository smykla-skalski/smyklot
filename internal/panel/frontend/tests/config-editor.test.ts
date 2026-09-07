// @vitest-environment jsdom
import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import ConfigEditor from '../src/lib/components/ConfigEditor.svelte';
import { CONFIG } from '../stories/support/fixtures';

describe('ConfigEditor drafts [Component]', () => {
  beforeEach(() => {
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  it.each(['runtime', 'target', 'repository'] as const)(
    'uses shared behavior choices and preserves explicit false until Reset in %s',
    async (scope) => {
      const onChange = vi.fn();
      render(ConfigEditor, {
        patch: {},
        inherited: { ...CONFIG, allow_draft_merges: false },
        scope,
        idPrefix: `choices-${scope}`,
        section: 'behavior',
        onChange,
      });
      const label = 'Merge draft pull requests';
      expect(screen.queryByRole('checkbox', { name: label })).toBeNull();
      expect(screen.queryByRole('button', { name: label })).toBeNull();
      await fireEvent.click(screen.getByRole('button', { name: 'Override another' }));
      const choice = screen.getByRole('button', { name: label });
      expect(choice.classList.contains('btn')).toBe(true);
      expect(choice.classList.contains('btn-add')).toBe(true);
      expect(choice.querySelector(':scope > .button-label')?.textContent).toBe(label);
      expect(choice.querySelector(':scope > svg')).not.toBeNull();
      await fireEvent.click(choice);
      expect(onChange).toHaveBeenLastCalledWith(
        { allow_draft_merges: false },
        'allow_draft_merges',
      );
      expect(screen.queryByRole('button', { name: label })).toBeNull();
      await fireEvent.click(screen.getByRole('checkbox', { name: label }));
      expect(onChange).toHaveBeenLastCalledWith({ allow_draft_merges: true }, 'allow_draft_merges');
      await fireEvent.click(screen.getByRole('checkbox', { name: label }));
      expect(onChange).toHaveBeenLastCalledWith(
        { allow_draft_merges: false },
        'allow_draft_merges',
      );
      await fireEvent.click(screen.getByRole('button', { name: 'Reset' }));
      expect(onChange).toHaveBeenLastCalledWith({}, 'allow_draft_merges');
      expect(screen.queryByRole('checkbox', { name: label })).toBeNull();
      await fireEvent.click(screen.getByRole('button', { name: 'Override another' }));
      expect(screen.getByRole('button', { name: label })).not.toBeNull();
    },
  );

  it('disables open behavior choices when edit permission is removed', async () => {
    const onChange = vi.fn();
    const view = render(ConfigEditor, {
      patch: {},
      inherited: CONFIG,
      scope: 'target',
      idPrefix: 'permission',
      section: 'behavior',
      onChange,
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Override another' }));
    await view.rerender({ disabled: true });
    expect(
      (screen.getByRole('button', { name: 'Merge draft pull requests' }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    await fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(onChange).not.toHaveBeenCalled();
    expect(
      (screen.getByRole('button', { name: 'Override another' }) as HTMLButtonElement).disabled,
    ).toBe(true);
  });

  it('reports staged changes synchronously with the changed key', async () => {
    const onChange = vi.fn();
    render(ConfigEditor, {
      patch: {},
      inherited: CONFIG,
      scope: 'target',
      idPrefix: 'staged',
      section: 'commands',
      onChange,
    });

    await fireEvent.input(screen.getByLabelText('Prefix'), { target: { value: '!' } });

    expect(onChange).toHaveBeenCalledOnce();
    expect(onChange).toHaveBeenCalledWith({ command_prefix: '!' }, 'command_prefix');
  });

  it('marks only the supplied staged rows as unsaved', () => {
    render(ConfigEditor, {
      patch: { quiet_success: true, command_prefix: '!' },
      inherited: CONFIG,
      scope: 'target',
      idPrefix: 'dirty',
      dirtyKeys: ['quiet_success'],
      onChange: vi.fn(),
    });

    expect(screen.getByLabelText('Success replies').closest('[data-unsaved]')).not.toBeNull();
    expect(screen.getByLabelText('Prefix').closest('[data-unsaved]')).toBeNull();
  });

  it('stages the draft-merge opt-in as a positive boolean', async () => {
    const onChange = vi.fn();
    render(ConfigEditor, {
      patch: { allow_draft_merges: false },
      inherited: CONFIG,
      scope: 'target',
      idPrefix: 'draft-merge',
      section: 'behavior',
      onChange,
    });

    await fireEvent.click(screen.getByLabelText('Merge draft pull requests'));

    expect(onChange).toHaveBeenCalledWith({ allow_draft_merges: true }, 'allow_draft_merges');
  });

  function aliases() {
    const onChange = vi.fn();
    const onValidity = vi.fn();
    render(ConfigEditor, {
      patch: {},
      inherited: { ...CONFIG, command_aliases: { ship: 'merge', lgtm: 'approve' } },
      scope: 'target',
      idPrefix: 'aliases',
      section: 'commands',
      onChange,
      onValidity,
    });
    return { onChange, onValidity };
  }

  it('renames an alias without duplicating its old key and restores the original map', async () => {
    const { onChange } = aliases();
    await fireEvent.input(screen.getByLabelText('Alias ship'), { target: { value: 'deploy' } });
    await fireEvent.blur(screen.getByLabelText('Alias ship'));
    expect(onChange).toHaveBeenLastCalledWith(
      { command_aliases: { deploy: 'merge', lgtm: 'approve' } },
      'command_aliases',
    );
    expect(screen.queryByLabelText('Alias ship')).toBeNull();
    await fireEvent.input(screen.getByLabelText('Alias deploy'), { target: { value: 'ship' } });
    await fireEvent.blur(screen.getByLabelText('Alias deploy'));
    expect(onChange).toHaveBeenLastCalledWith({}, 'command_aliases');
  });

  it('does not overwrite an existing alias and clears a late error after correction', async () => {
    const { onChange, onValidity } = aliases();
    const key = screen.getByLabelText('Alias ship');
    await fireEvent.input(key, { target: { value: 'lgtm' } });
    expect(screen.queryByRole('alert')).toBeNull();
    await fireEvent.blur(key);
    expect(screen.getByRole('alert').textContent).toBe('That alias already exists');
    expect(onValidity).toHaveBeenLastCalledWith('That alias already exists');
    expect(onChange).not.toHaveBeenCalled();
    await fireEvent.input(key, { target: { value: 'ship' } });
    expect(screen.queryByRole('alert')).toBeNull();
    expect(onValidity).toHaveBeenLastCalledWith(null);
  });

  it('filters valid commands and can return to the original selection', async () => {
    const { onChange } = aliases();
    const command = screen.getByRole('combobox', { name: 'Command for alias ship' });
    await fireEvent.focus(command);
    await fireEvent.input(command, { target: { value: 'squ' } });
    expect(screen.getAllByRole('option')).toHaveLength(1);
    await fireEvent.pointerUp(screen.getByRole('option', { name: /squash/ }));
    expect(onChange).toHaveBeenLastCalledWith(
      { command_aliases: { ship: 'squash', lgtm: 'approve' } },
      'command_aliases',
    );
    await fireEvent.focus(command);
    await fireEvent.pointerUp(screen.getByRole('option', { name: /^merge / }));
    expect(onChange).toHaveBeenLastCalledWith({}, 'command_aliases');
  });

  it('reopens all command choices when the already-focused input is pressed', async () => {
    const { onChange } = aliases();
    const command = screen.getByRole('combobox', { name: 'Command for alias ship' });
    await fireEvent.focus(command);
    await fireEvent.keyDown(command, { key: 'Escape' });
    expect(command.getAttribute('aria-expanded')).toBe('false');
    await fireEvent.click(command);
    expect(command.getAttribute('aria-expanded')).toBe('true');
    expect(screen.getAllByRole('option')).toHaveLength(7);
    expect(onChange).not.toHaveBeenCalled();
  });

  it('requires selecting a command when adding and remove/re-add restores inheritance', async () => {
    const { onChange } = aliases();
    await fireEvent.click(screen.getByRole('button', { name: 'Remove alias ship' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Add an alias' }));
    const key = screen.getByLabelText('Name for the new alias');
    await fireEvent.input(key, { target: { value: 'ship' } });
    await fireEvent.blur(key);
    expect(onChange).toHaveBeenCalledTimes(1);
    await fireEvent.focus(screen.getByRole('combobox', { name: 'Command for the new alias' }));
    await fireEvent.pointerUp(screen.getByRole('option', { name: /^merge / }));
    expect(onChange).toHaveBeenLastCalledWith({}, 'command_aliases');
    expect(screen.queryByLabelText('Name for the new alias')).toBeNull();
  });

  it('resets aliases as a group and can return a command checklist to its original set', async () => {
    const { onChange } = aliases();
    await fireEvent.click(screen.getByRole('button', { name: 'Remove alias ship' }));
    const row = screen
      .getByRole('group', { name: 'Aliases' })
      .closest('.policy-row')! as HTMLElement;
    await fireEvent.click(within(row).getByRole('button', { name: 'Reset' }));
    expect(onChange).toHaveBeenLastCalledWith({}, 'command_aliases');
    const check = screen.getByRole('checkbox', { name: 'merge' });
    await fireEvent.click(check);
    await fireEvent.click(check);
    expect(onChange).toHaveBeenLastCalledWith({}, 'allowed_commands');
  });
});

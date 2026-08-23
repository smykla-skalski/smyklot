// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import ConfigEditor from '../src/lib/components/ConfigEditor.svelte';
import { CONFIG } from '../stories/support/fixtures';

describe('ConfigEditor save modes [Component]', () => {
  beforeEach(() => {
    document.body.innerHTML = '<main class="app-shell"></main>';
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

  it('keeps the legacy save callback for settings surfaces still being migrated', async () => {
    const onSave = vi.fn(async () => {});
    render(ConfigEditor, {
      patch: {},
      inherited: CONFIG,
      scope: 'runtime',
      idPrefix: 'legacy',
      section: 'commands',
      onSave,
    });

    const prefix = screen.getByLabelText('Prefix');
    await fireEvent.input(prefix, { target: { value: '?' } });
    expect(onSave).not.toHaveBeenCalled();
    await fireEvent.blur(prefix);

    expect(onSave).toHaveBeenCalledOnce();
    expect(onSave).toHaveBeenCalledWith({ command_prefix: '?' });
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
});

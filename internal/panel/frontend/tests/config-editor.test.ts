// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import ConfigEditor from '../src/lib/components/ConfigEditor.svelte';
import { CONFIG } from '../stories/support/fixtures';

describe('ConfigEditor drafts [Component]', () => {
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
});

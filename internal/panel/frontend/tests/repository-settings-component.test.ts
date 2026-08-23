// @vitest-environment jsdom
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import RepositorySettings from '../src/lib/components/RepositorySettings.svelte';
import type {
  RepositorySettingsControlId,
  RepositorySettingsDocument,
} from '../src/lib/repository-settings';
import { REPOSITORY, REPOSITORY_DETAIL } from '../stories/support/fixtures';

class TestResizeObserver {
  observe(): void {}
  disconnect(): void {}
}

function base(over: Record<string, unknown> = {}) {
  return {
    repository: REPOSITORY,
    detail: REPOSITORY_DETAIL,
    section: 'behavior' as const,
    backHref: '/repositories',
    onBack: () => {},
    onSection: () => {},
    onChange: () => {},
    onResetMigration: () => {},
    ...over,
  };
}

describe('RepositorySettings shared drafts [Component]', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', TestResizeObserver);
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  it('stages a complete numeric edit synchronously under its stable control', async () => {
    const changes: Array<{
      document: RepositorySettingsDocument;
      controls: readonly RepositorySettingsControlId[];
    }> = [];
    render(RepositorySettings, {
      ...base({
        onChange: (
          document: RepositorySettingsDocument,
          controls: readonly RepositorySettingsControlId[],
        ) => changes.push({ document, controls }),
      }),
    });

    const quiet = screen.getByLabelText('Stable passing window') as HTMLInputElement;
    await fireEvent.input(quiet, { target: { value: '45' } });

    expect(changes).toHaveLength(1);
    expect(changes[0].document.pending_ci_quiet_period_seconds_override).toBe(45);
    expect(changes[0].controls).toEqual([
      `repositories.${REPOSITORY.id}.pending_ci_quiet_period_seconds_override`,
    ]);
  });

  it('keeps ordinary controls editable while the migration reset command is busy', () => {
    render(RepositorySettings, { ...base({ busy: true }) });

    expect((screen.getByLabelText('Stable passing window') as HTMLInputElement).disabled).toBe(
      false,
    );
  });

  it('marks the exact dirty row without adding a save receipt', () => {
    const control = `repositories.${REPOSITORY.id}.pending_ci_quiet_period_seconds_override`;
    render(RepositorySettings, { ...base({ dirtyControls: [control] }) });

    const quiet = screen.getByLabelText('Stable passing window');
    expect(quiet.closest('[data-unsaved]')?.getAttribute('data-unsaved')).toBe('true');
    expect(screen.queryByText('Saved')).toBeNull();
  });

  it('contains no immediate repository write, debounce, or receipt path', () => {
    const settings = readFileSync(
      resolve(process.cwd(), 'src/lib/components/RepositorySettings.svelte'),
      'utf8',
    );
    const list = readFileSync(
      resolve(process.cwd(), 'src/lib/components/RepositoryList.svelte'),
      'utf8',
    );
    expect(settings).not.toContain('setTimeout');
    expect(settings).not.toContain('save-whisper');
    expect(list).not.toContain('onUpdate');
    expect(list).not.toContain('onSaveSyncOverride');
  });
});

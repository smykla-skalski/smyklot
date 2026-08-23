// @vitest-environment jsdom
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it } from 'vitest';

import { SettingsDraftRegistry } from '../src/lib/settings-drafts.svelte';
import {
  targetDefaultsDraftDocument,
  targetDefaultsResource,
} from '../src/lib/target-defaults-settings';
import { TARGET } from '../stories/support/fixtures';
import TargetSettingsHarness from './support/TargetSettingsHarness.svelte';

function registry(): SettingsDraftRegistry {
  const drafts = new SettingsDraftRegistry({ storage: null, now: () => 1, writerId: 'test' });
  drafts.hydrate('viewer-1');
  return drafts;
}

describe('TargetSettings shared drafts [Component]', () => {
  beforeEach(() => {
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  it('adopts the canonical target and stages a control without a save callback', async () => {
    const drafts = registry();
    const target = { ...TARGET, repository_default_enabled: false };
    render(TargetSettingsHarness, { props: { drafts, target } });

    await waitFor(() => expect(drafts.resource(targetDefaultsResource(target.id))).not.toBeNull());
    const toggle = screen.getByLabelText('Unconfigured repositories') as HTMLInputElement;
    expect(toggle.checked).toBe(false);

    await fireEvent.click(toggle);

    expect(targetDefaultsDraftDocument(drafts, target).repository_default_enabled).toBe(true);
    expect(drafts.dirtyControls()).toMatchObject([
      { id: 'defaults.repository_default_enabled', saved: false, value: true },
    ]);
    expect(toggle.closest('[data-unsaved]')?.getAttribute('data-unsaved')).toBe('true');

    expect(drafts.discardResource(targetDefaultsResource(target.id))).toBe(true);
    await waitFor(() => expect(toggle.checked).toBe(false));
    expect(toggle.closest('[data-unsaved]')).toBeNull();
  });

  it('keeps every setting disabled for a read-only viewer', async () => {
    const drafts = registry();
    render(TargetSettingsHarness, { props: { drafts, target: TARGET, readOnly: true } });

    await waitFor(() => expect(drafts.resource(targetDefaultsResource(TARGET.id))).not.toBeNull());
    expect((screen.getByLabelText('Unconfigured repositories') as HTMLInputElement).disabled).toBe(
      true,
    );
    expect((screen.getByLabelText('Stable passing window') as HTMLInputElement).disabled).toBe(
      true,
    );
    expect((screen.getByLabelText('Prefix') as HTMLInputElement).disabled).toBe(true);
  });

  it('keeps multi-digit numeric text stable and editable during an in-flight save', async () => {
    const drafts = registry();
    render(TargetSettingsHarness, { props: { drafts, target: TARGET } });
    await waitFor(() => expect(drafts.resource(targetDefaultsResource(TARGET.id))).not.toBeNull());

    const quiet = screen.getByLabelText('Stable passing window') as HTMLInputElement;
    await fireEvent.input(quiet, { target: { value: '1' } });
    const attempt = drafts.beginSave({ type: 'installation', targetId: TARGET.id });
    expect(attempt).not.toBeNull();
    expect(quiet.disabled).toBe(false);

    await fireEvent.input(quiet, { target: { value: '12' } });
    expect(quiet.value).toBe('12');
    await fireEvent.input(quiet, { target: { value: '120' } });
    expect(quiet.value).toBe('120');
    expect(
      targetDefaultsDraftDocument(drafts, TARGET).pending_ci_quiet_period_seconds_override,
    ).toBe(120);
    expect(drafts.dirtyControls().map(({ id }) => id)).toEqual([
      'defaults.pending_ci_quiet_period_seconds_override',
    ]);

    await fireEvent.blur(quiet);
    expect(quiet.value).toBe('120');
  });

  it('contains no immediate target save callback, debounce, or receipt path', () => {
    const component = readFileSync(
      resolve(process.cwd(), 'src/lib/components/TargetSettings.svelte'),
      'utf8',
    );
    expect(component).not.toContain('onUpdate');
    expect(component).not.toContain('setTimeout');
    expect(component).not.toContain('save-whisper');
  });
});

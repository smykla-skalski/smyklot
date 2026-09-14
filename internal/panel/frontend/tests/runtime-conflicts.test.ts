import { describe, expect, it } from 'vitest';
import { PanelApiError } from '../src/lib/api';
import { rebaseRootSettingsConflict, saveRootSettingsDraft } from '../src/lib/root-settings-save';
import { runtimeConflictValue, runtimeFieldConflicts } from '../src/lib/runtime-conflicts';
import {
  adoptRuntimeSettings,
  applyRuntimeConfigPatch,
  runtimeSettingsDraftDocument,
  stageRuntimeSettingsControl,
} from '../src/lib/runtime-settings';
import { SettingsDraftRegistry } from '../src/lib/settings-drafts.svelte';
import { RUNTIME } from '../stories/support/fixtures';

async function conflict(remotePrefix: string | null) {
  const current = structuredClone(RUNTIME);
  current.behavior_defaults.intent = null;
  current.behavior_defaults.override = null;
  const registry = new SettingsDraftRegistry({ storage: null, writerId: 'conflict' });
  registry.hydrate('viewer');
  adoptRuntimeSettings(registry, current);
  stageRuntimeSettingsControl(
    registry,
    current,
    {
      ...runtimeSettingsDraftDocument(registry, current),
      bot_config: applyRuntimeConfigPatch(current.behavior_defaults.deployment, {
        command_prefix: '/mine',
      }),
    },
    'runtime.bot_config.command_prefix',
  );
  stageRuntimeSettingsControl(
    registry,
    current,
    { ...runtimeSettingsDraftDocument(registry, current), log_level: 'debug' },
    'runtime.log_level',
  );
  const latest = structuredClone(current);
  latest.revision++;
  latest.behavior_defaults.intent =
    remotePrefix === null ? null : { version: 1, overrides: { command_prefix: remotePrefix } };
  latest.session_lifetime.override_seconds = 3600;
  await saveRootSettingsDraft(
    registry,
    async () => latest,
    async () => {
      throw new PanelApiError(409, 'conflict', 'Changed');
    },
  );
  return { registry, latest };
}

describe('runtime field conflict choices [Unit]', () => {
  it('requires a decision for a field changed differently in both sessions', async () => {
    const { registry, latest } = await conflict('/remote');
    expect(runtimeFieldConflicts(registry, latest)).toEqual([
      {
        id: 'runtime.bot_config.command_prefix',
        label: 'Command prefix',
        draft: '/mine',
        saved: '/remote',
      },
    ]);
    const before = runtimeSettingsDraftDocument(registry, latest);
    expect(rebaseRootSettingsConflict(registry, latest)).toBe(false);
    expect(runtimeSettingsDraftDocument(registry, latest)).toEqual(before);
  });
  it.each(['draft', 'saved'] as const)('keeps unrelated edits when choosing %s', async (choice) => {
    const { registry, latest } = await conflict('/remote');
    expect(
      rebaseRootSettingsConflict(registry, latest, { 'runtime.bot_config.command_prefix': choice }),
    ).toBe(true);
    const result = runtimeSettingsDraftDocument(registry, latest);
    expect(result.bot_config?.overrides.command_prefix).toBe(
      choice === 'draft' ? '/mine' : '/remote',
    );
    expect(result.log_level).toBe('debug');
    expect(result.session_ttl_seconds.override_seconds).toBe(3600);
    expect(runtimeFieldConflicts(registry, latest)).toEqual([]);
  });
  it.each([null, '/mine'])(
    'rebases unrelated or identical changes without a choice: %s',
    async (prefix) => {
      const { registry, latest } = await conflict(prefix);
      expect(runtimeFieldConflicts(registry, latest)).toEqual([]);
      expect(rebaseRootSettingsConflict(registry, latest)).toBe(true);
      expect(
        runtimeSettingsDraftDocument(registry, latest).bot_config?.overrides.command_prefix,
      ).toBe('/mine');
    },
  );
  it('requires all decisions and becomes clean when every saved value wins', async () => {
    const { registry, latest } = await conflict('/remote');
    latest.log_level.override = 'warn';
    expect(runtimeFieldConflicts(registry, latest)).toHaveLength(2);
    expect(
      rebaseRootSettingsConflict(registry, latest, {
        'runtime.bot_config.command_prefix': 'saved',
      }),
    ).toBe(false);
    expect(
      rebaseRootSettingsConflict(registry, latest, {
        'runtime.bot_config.command_prefix': 'saved',
        'runtime.log_level': 'saved',
      }),
    ).toBe(true);
    expect(registry.dirty).toBe(false);
  });
  it('describes ownership and suppression booleans in user terms', () => {
    expect(runtimeConflictValue('runtime.bot_config.quiet_success', false)).toBe('On');
    expect(runtimeConflictValue('runtime.bot_config.quiet_success', true)).toBe('Off');
    expect(runtimeConflictValue('runtime.bot_config.quiet_success', null)).toBe(
      'Follow deployment',
    );
  });
});

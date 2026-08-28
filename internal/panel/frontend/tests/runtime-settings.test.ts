import { describe, expect, it, vi } from 'vitest';

import { PanelApiError } from '../src/lib/api';
import { applyFormattingPatch } from '../src/lib/formatting';
import { rebaseRootSettingsConflict, saveRootSettingsDraft } from '../src/lib/root-settings-save';
import {
  adoptRuntimeSettings,
  buildRuntimeSettingsDraftDocument,
  parseRuntimeSettingsDraftDocument,
  RUNTIME_DURATION_SPECS,
  RUNTIME_RESOURCE,
  ROOT_SETTINGS_SCOPE,
  runtimeSettingsDraftDocument,
  serializeRuntimeSettingsDraft,
  stageRuntimeSettingsControl,
} from '../src/lib/runtime-settings';
import { SettingsDraftRegistry, settingsDraftStorageKey } from '../src/lib/settings-drafts.svelte';
import type { SettingsDraftStorage } from '../src/lib/settings-draft-storage';
import type { RootRuntimeSettings, RootRuntimeSettingsInput } from '../src/lib/types';
import { RUNTIME } from '../stories/support/fixtures';

function memoryStorage(): SettingsDraftStorage & { value(key: string): string | null } {
  const values = new Map<string, string>();
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
    value: (key) => values.get(key) ?? null,
  };
}

function registry(storage: SettingsDraftStorage | null = null): SettingsDraftRegistry {
  const drafts = new SettingsDraftRegistry({ storage, now: () => 10, writerId: 'test' });
  drafts.hydrate('viewer');
  return drafts;
}

function runtime(over: Partial<RootRuntimeSettings> = {}): RootRuntimeSettings {
  return structuredClone({ ...RUNTIME, ...over });
}

describe('Root runtime settings drafts [Unit]', () => {
  it('round-trips a generated preset through the complete runtime config', () => {
    const drafts = registry();
    const current = runtime();
    adoptRuntimeSettings(drafts, current);
    const document = runtimeSettingsDraftDocument(drafts, current);
    const formatting = applyFormattingPatch(current.behavior_defaults.deployment.formatting, {
      preset: 'conventional',
    });
    const next = {
      ...document,
      bot_config: { ...current.behavior_defaults.deployment, formatting },
    };

    expect(
      stageRuntimeSettingsControl(drafts, current, next, 'runtime.bot_config.formatting.preset'),
    ).toBe(true);
    const serialized = serializeRuntimeSettingsDraft(
      current.revision,
      runtimeSettingsDraftDocument(drafts, current),
    );

    expect(serialized.ok).toBe(true);
    if (!serialized.ok) return;
    expect(serialized.input.bot_config?.formatting).toEqual(formatting);
    expect(serialized.input.bot_config?.formatting.json.arrays).toBe('auto');
  });

  it('rejects invalid complete formatting policies before serialization', () => {
    const current = runtime();
    const document = buildRuntimeSettingsDraftDocument(current);
    const invalid = {
      ...document,
      bot_config: {
        ...current.behavior_defaults.deployment,
        formatting: {
          ...current.behavior_defaults.deployment.formatting,
          json: { arrays: 'wide' },
        },
      },
    };

    expect(parseRuntimeSettingsDraftDocument(invalid)).toBeNull();
  });

  it('hydrates legacy full-config documents with the safe draft-merge default', () => {
    const current = runtime({
      behavior_defaults: {
        deployment: RUNTIME.behavior_defaults.deployment,
        override: { ...RUNTIME.behavior_defaults.deployment },
        effective: RUNTIME.behavior_defaults.effective,
      },
    });
    const legacy = buildRuntimeSettingsDraftDocument(current);
    delete (legacy.bot_config as Record<string, unknown>).allow_draft_merges;

    const parsed = parseRuntimeSettingsDraftDocument(legacy);

    expect(parsed?.bot_config?.allow_draft_merges).toBe(false);
  });

  it('persists bounded raw duration input and refuses it before the wire', () => {
    const storage = memoryStorage();
    const first = registry(storage);
    const current = runtime();
    expect(adoptRuntimeSettings(first, current)).toBe(true);
    const document = runtimeSettingsDraftDocument(first, current);
    expect(
      stageRuntimeSettingsControl(
        first,
        current,
        {
          ...document,
          merge_after_ci_quiet_period_seconds: {
            override_seconds: current.merge_after_ci_quiet_period.deployment_seconds,
            editor: { amount: '1e', unit: 'minutes' },
          },
        },
        'runtime.merge_after_ci_quiet_period_seconds',
      ),
    ).toBe(true);

    const restarted = registry(storage);
    const restored = parseRuntimeSettingsDraftDocument(restarted.value(RUNTIME_RESOURCE));
    expect(restored?.merge_after_ci_quiet_period_seconds.editor).toEqual({
      amount: '1e',
      unit: 'minutes',
    });
    expect(serializeRuntimeSettingsDraft(current.revision, restored!)).toEqual({
      ok: false,
      controlId: 'runtime.merge_after_ci_quiet_period_seconds',
      problem: RUNTIME_DURATION_SPECS.merge_after_ci_quiet_period_seconds.problem,
    });
    expect(storage.value(settingsDraftStorageKey('viewer'))).toContain('1e');
  });

  it('sends one complete settings request and commits the checkpoint response', async () => {
    const drafts = registry();
    const current = runtime();
    adoptRuntimeSettings(drafts, current);
    const document = runtimeSettingsDraftDocument(drafts, current);
    stageRuntimeSettingsControl(
      drafts,
      current,
      { ...document, log_level: 'debug' },
      'runtime.log_level',
    );
    const afterLog = runtimeSettingsDraftDocument(drafts, current);
    stageRuntimeSettingsControl(
      drafts,
      current,
      {
        ...afterLog,
        merge_after_ci_quiet_period_seconds: {
          override_seconds: null,
          editor: { amount: '2', unit: 'minutes' },
        },
      },
      'runtime.merge_after_ci_quiet_period_seconds',
    );

    const updated = runtime({
      revision: current.revision + 1,
      checkpoint_id: 'checkpoint-2',
      log_level: { ...current.log_level, override: 'debug', effective: 'debug' },
      merge_after_ci_quiet_period: {
        ...current.merge_after_ci_quiet_period,
        override_seconds: 120,
        effective_seconds: 120,
      },
    });
    const save = vi.fn(async (input: RootRuntimeSettingsInput) => {
      void input;
      return updated;
    });
    const fetch = vi.fn(async () => current);
    const result = await saveRootSettingsDraft(drafts, fetch, save);

    expect(fetch).not.toHaveBeenCalled();
    expect(save).toHaveBeenCalledOnce();
    expect(save.mock.calls[0]?.[0]).toMatchObject({
      bot_config: null,
      log_level: 'debug',
      reaction_poll_interval_seconds: null,
      merge_after_ci_quiet_period_seconds: 120,
      path_index_interval_seconds: null,
      session_ttl_seconds: null,
      expected_revision: current.revision,
    });
    expect(result).toMatchObject({ saved: true, checkpointId: 'checkpoint-2' });
    expect(drafts.hasDirty(ROOT_SETTINGS_SCOPE)).toBe(false);
  });

  it('preserves edits across a revision conflict and rebases them explicitly', async () => {
    const drafts = registry();
    const current = runtime();
    adoptRuntimeSettings(drafts, current);
    const document = runtimeSettingsDraftDocument(drafts, current);
    stageRuntimeSettingsControl(
      drafts,
      current,
      { ...document, log_level: 'debug' },
      'runtime.log_level',
    );
    const afterLog = runtimeSettingsDraftDocument(drafts, current);
    const wantedFormatting = applyFormattingPatch(current.behavior_defaults.deployment.formatting, {
      preset: 'conventional',
    });
    stageRuntimeSettingsControl(
      drafts,
      current,
      {
        ...afterLog,
        bot_config: {
          ...current.behavior_defaults.deployment,
          formatting: wantedFormatting,
        },
      },
      'runtime.bot_config.formatting.preset',
    );
    const concurrentFormatting = applyFormattingPatch(
      current.behavior_defaults.deployment.formatting,
      { json: { arrays: 'expanded' } },
    );
    const concurrentConfig = {
      ...current.behavior_defaults.deployment,
      formatting: concurrentFormatting,
    };
    const latest = runtime({
      revision: current.revision + 1,
      behavior_defaults: {
        deployment: current.behavior_defaults.deployment,
        override: concurrentConfig,
        effective: concurrentConfig,
      },
      session_lifetime: {
        ...current.session_lifetime,
        override_seconds: 3_600,
        effective_seconds: 3_600,
      },
    });

    const result = await saveRootSettingsDraft(
      drafts,
      async () => latest,
      async () => {
        throw new PanelApiError(409, 'conflict', 'Runtime settings changed in another session');
      },
    );
    expect(result.saved).toBe(false);
    expect(drafts.hasConflicts(ROOT_SETTINGS_SCOPE)).toBe(true);

    expect(rebaseRootSettingsConflict(drafts, latest)).toBe(true);
    const rebased = runtimeSettingsDraftDocument(drafts, latest);
    expect(rebased.log_level).toBe('debug');
    expect(rebased.session_ttl_seconds.override_seconds).toBe(3_600);
    expect(rebased.bot_config?.formatting).toEqual(wantedFormatting);
    expect(rebased.bot_config?.formatting.json.arrays).toBe('auto');
    expect(drafts.hasConflicts(ROOT_SETTINGS_SCOPE)).toBe(false);
  });
});

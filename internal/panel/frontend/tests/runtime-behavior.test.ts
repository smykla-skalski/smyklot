import { describe, expect, it } from 'vitest';
import { RUNTIME } from '../stories/support/fixtures';
import {
  parseRuntimeBehavior,
  resolveRuntimeBehavior,
  runtimeBehaviorFromPatch,
} from '../src/lib/runtime-behavior';
import {
  adoptRuntimeSettings,
  applyRuntimeConfigPatch,
  runtimeSettingsDraftDocument,
  serializeRuntimeSettingsDraft,
  stageRuntimeSettingsControl,
} from '../src/lib/runtime-settings';
import { SettingsDraftRegistry } from '../src/lib/settings-drafts.svelte';

describe('runtime behavior ownership [Unit]', () => {
  it('keeps explicit false and empty values while omitted fields follow deployment', () => {
    const input = {
      version: 1,
      overrides: {
        quiet_success: false,
        command_prefix: '',
        allowed_commands: [],
        command_aliases: {},
      },
    };
    const intent = parseRuntimeBehavior(input);
    expect(intent).toEqual(input);
    const deployment = structuredClone(RUNTIME.behavior_defaults.deployment);
    deployment.quiet_success = true;
    deployment.quiet_pending = true;
    expect(resolveRuntimeBehavior(deployment, intent)).toMatchObject({
      quiet_success: false,
      quiet_pending: true,
      command_prefix: '',
      allowed_commands: [],
      command_aliases: {},
    });
    input.overrides.quiet_success = true;
    expect(intent?.overrides.quiet_success).toBe(false);
  });

  it('stages and serializes an equal-value override as real user intent', () => {
    const current = structuredClone(RUNTIME);
    current.behavior_defaults.intent = null;
    current.behavior_defaults.override = null;
    const registry = new SettingsDraftRegistry({ storage: null, writerId: 'ownership' });
    registry.hydrate('viewer');
    adoptRuntimeSettings(registry, current);
    const document = runtimeSettingsDraftDocument(registry, current);
    const desired = current.behavior_defaults.deployment.quiet_success;
    expect(
      stageRuntimeSettingsControl(
        registry,
        current,
        {
          ...document,
          bot_config: applyRuntimeConfigPatch({
            quiet_success: desired,
          }),
        },
        'runtime.bot_config.quiet_success',
      ),
    ).toBe(true);
    const held = runtimeSettingsDraftDocument(registry, current);
    expect(serializeRuntimeSettingsDraft(current.revision, held)).toMatchObject({
      ok: true,
      input: { bot_config: { version: 1, overrides: { quiet_success: desired } } },
    });
  });

  it('preserves formatting leaf ownership and removes empty groups', () => {
    const intent = runtimeBehaviorFromPatch({
      formatting: { common: { line_ending: 'preserve' } },
    });
    expect(intent).toEqual({
      version: 1,
      overrides: { formatting: { common: { line_ending: 'preserve' } } },
    });
    expect(runtimeBehaviorFromPatch({ formatting: { common: {} } })).toBeNull();
    expect(runtimeBehaviorFromPatch({ formatting: {}, quiet_success: false })).toEqual({
      version: 1,
      overrides: { quiet_success: false },
    });
  });

  it('copies reactive proxies without relying on structuredClone', () => {
    const list = new Proxy(['approve'], {});
    const intent = runtimeBehaviorFromPatch({ allowed_commands: list });
    list.push('merge');
    expect(intent?.overrides.allowed_commands).toEqual(['approve']);
  });

  it('retains complete legacy ownership when deployment values change', () => {
    const legacy = structuredClone(RUNTIME.behavior_defaults.deployment);
    const intent = parseRuntimeBehavior(legacy);
    const changed = {
      ...legacy,
      quiet_success: !legacy.quiet_success,
      quiet_pending: !legacy.quiet_pending,
    };
    expect(resolveRuntimeBehavior(changed, intent)).toEqual(legacy);
  });

  it('pins legacy zero values without requiring fields added by later versions', () => {
    const intent = parseRuntimeBehavior({ quiet_success: true });
    const resolved = resolveRuntimeBehavior(RUNTIME.behavior_defaults.deployment, intent);
    expect(resolved).toMatchObject({
      quiet_success: true,
      quiet_pending: false,
      allow_draft_merges: false,
      command_prefix: '',
      allowed_commands: [],
      command_aliases: {},
    });
    expect(intent?.overrides.formatting?.preset).toBe('preserve');
  });

  it.each(['unknown', 1, false])('rejects an invalid legacy runner %j', (runner) => {
    expect(() =>
      parseRuntimeBehavior({ ...RUNTIME.behavior_defaults.deployment, runner }),
    ).toThrow();
  });

  it.each([
    { version: 2, overrides: {} },
    { version: 1, overrides: {}, extra: true },
    { version: 1, overrides: null },
    { version: 1, overrides: { runner: 'service' } },
    { version: 1, overrides: { unknown: true } },
    { version: 1, overrides: { quiet_success: null } },
    { version: 1, overrides: { quiet_success: 'false' } },
    { version: 1, overrides: { formatting: { common: { line_ending: null } } } },
  ])('rejects malformed intent %j', (value) => {
    expect(() => parseRuntimeBehavior(value)).toThrow();
  });
});

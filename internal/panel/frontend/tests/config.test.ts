import { describe, expect, it } from 'vitest';

import {
  clonePatch,
  commandIsAllowed,
  countOverrides,
  effectiveValue,
  patchesEqual,
  reconcilePatchDraft,
  setExplicitPatchValue,
  toggleAllowedCommand,
  updatePatchValue,
} from '../src/lib/config';
import type { ConfigValues } from '../src/lib/types';

const EFFECTIVE: ConfigValues = {
  quiet_success: false,
  quiet_reactions: false,
  quiet_pending: false,
  allowed_commands: ['approve'],
  command_aliases: { ship: 'merge' },
  command_prefix: '/',
  disable_mentions: false,
  disable_bare_commands: false,
  disable_unapprove: false,
  disable_reactions: false,
  disable_deleted_comments: false,
  allow_self_approval: false,
};

describe('configuration patches', () => {
  it('presents an empty allowed-command list as every command enabled', () => {
    expect(commandIsAllowed([], 'approve')).toBe(true);
    expect(commandIsAllowed(['merge'], 'approve')).toBe(false);
    expect(commandIsAllowed(['merge'], 'merge')).toBe(true);
  });

  it('keeps all-command storage compact while toggling semantic selections', () => {
    const commands = ['approve', 'merge', 'cleanup'] as const;

    expect(toggleAllowedCommand([], 'merge', commands)).toEqual(['approve', 'cleanup']);
    expect(toggleAllowedCommand(['approve', 'cleanup'], 'merge', commands)).toEqual([]);
    expect(toggleAllowedCommand(['approve'], 'approve', commands)).toEqual(['approve']);
  });

  it('distinguishes inherited values from explicit false and empty collections', () => {
    const patch = { quiet_success: false, allowed_commands: [], command_aliases: {} };
    expect(countOverrides(patch)).toBe(3);
    expect(effectiveValue(patch, EFFECTIVE, 'quiet_success')).toBe(false);
    expect(effectiveValue(patch, EFFECTIVE, 'allowed_commands')).toEqual([]);
  });

  it('keeps an explicit boolean override even when it matches inheritance', () => {
    expect(setExplicitPatchValue({}, 'quiet_success', false)).toEqual({ quiet_success: false });
    expect(setExplicitPatchValue({ quiet_success: true }, 'quiet_success', false)).toEqual({
      quiet_success: false,
    });
  });

  it('preserves an unsaved draft across an unrelated target refresh', () => {
    const baseline = { quiet_success: true };
    const draft = { quiet_success: false };

    expect(reconcilePatchDraft(draft, baseline, { quiet_success: true })).toBe(draft);
    expect(
      reconcilePatchDraft(draft, baseline, {
        quiet_success: true,
        disable_mentions: true,
      }),
    ).toEqual({ quiet_success: true, disable_mentions: true });
  });

  it('clones nested values and compares patches independent of key order', () => {
    const original = { command_aliases: { ship: 'merge' }, allowed_commands: ['merge', 'approve'] };
    const cloned = clonePatch(original);
    cloned.command_aliases = { changed: 'help' };

    expect(original.command_aliases).toEqual({ ship: 'merge' });
    expect(
      patchesEqual(original, {
        allowed_commands: ['approve', 'merge'],
        command_aliases: { ship: 'merge' },
      }),
    ).toBe(true);
  });

  it('adds overrides for changed values and removes them when they match inheritance', () => {
    const customPrefix = updatePatchValue({}, EFFECTIVE, 'command_prefix', '!');
    expect(customPrefix).toEqual({ command_prefix: '!' });
    expect(updatePatchValue(customPrefix, EFFECTIVE, 'command_prefix', '/')).toEqual({});

    const customCommands = updatePatchValue({}, EFFECTIVE, 'allowed_commands', [
      'merge',
      'approve',
    ]);
    expect(customCommands).toEqual({ allowed_commands: ['merge', 'approve'] });
    expect(updatePatchValue(customCommands, EFFECTIVE, 'allowed_commands', ['approve'])).toEqual(
      {},
    );

    const customAliases = updatePatchValue({}, EFFECTIVE, 'command_aliases', {
      ship: 'squash',
    });
    expect(customAliases).toEqual({ command_aliases: { ship: 'squash' } });
    expect(
      updatePatchValue(customAliases, EFFECTIVE, 'command_aliases', { ship: 'merge' }),
    ).toEqual({});

    const customBoolean = updatePatchValue({}, EFFECTIVE, 'quiet_reactions', true);
    expect(customBoolean).toEqual({ quiet_reactions: true });
    expect(updatePatchValue(customBoolean, EFFECTIVE, 'quiet_reactions', false)).toEqual({});
  });
});

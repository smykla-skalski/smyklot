import { describe, expect, it } from 'vitest';

import { CONFIG_KEYS } from '../src/lib/config';
import {
  adoptRepositorySettings,
  buildRepositorySettingsDocument,
  overlayRepositorySettingsDocument,
  parseRepositorySettingsDocument,
  repositorySettingsBatchInput,
  repositorySettingsCommittedResource,
  repositorySettingsControls,
  repositorySettingsDraftDocument,
  repositorySettingsResource,
  repositorySettingsSavedControls,
  stageRepositorySettingsControl,
} from '../src/lib/repository-settings';
import { SettingsDraftRegistry } from '../src/lib/settings-drafts.svelte';
import type {
  ConfigSources,
  ConfigValues,
  InstallationRepositorySettingsState,
  RepositoryDetail,
} from '../src/lib/types';

const VALUES: ConfigValues = {
  quiet_success: false,
  quiet_reactions: false,
  quiet_pending: false,
  allowed_commands: [],
  command_aliases: {},
  command_prefix: '/',
  disable_mentions: false,
  disable_bare_commands: false,
  disable_unapprove: false,
  disable_reactions: false,
  disable_deleted_comments: false,
  allow_self_approval: false,
  allow_draft_merges: false,
};

const SOURCES = Object.fromEntries(CONFIG_KEYS.map((key) => [key, 'target'])) as ConfigSources;

function repository(): RepositoryDetail {
  return {
    repository: {
      id: 'repo-1',
      name: 'api',
      full_name: 'example/api',
      private: true,
      default_branch: 'main',
      available: true,
      enabled_override: false,
      effective_enabled: false,
      enabled_source: 'repository',
      pending_ci_mode: 'labels',
      pending_ci_mode_source: 'repository',
      config_override_count: 3,
      config_file_status: 'valid',
      updated_at: '2026-08-23T10:00:00Z',
    },
    config_patch: {
      quiet_success: false,
      allowed_commands: [],
      command_aliases: { ship: 'merge' },
    },
    inherited_config: { ...VALUES },
    effective_config: { ...VALUES },
    config_sources: { ...SOURCES },
    config_file_patch: {},
    config_migration: 'none',
    ignore_repository_file: true,
    pending_ci_mode_override: 'labels',
    pending_ci_mode_inherited: 'checks',
    pending_ci_branch_patterns_override: { include: [], exclude: [] },
    pending_ci_branch_patterns_inherited: { include: ['~DEFAULT_BRANCH'], exclude: [] },
    pending_ci_quiet_period_seconds_override: 45,
    pending_ci_quiet_period_seconds_inherited: 30,
    path_index_interval_seconds_override: 3_600,
    path_index_interval_seconds_inherited: 900,
    revision: 7,
  };
}

describe('repository settings adapter [Unit]', () => {
  it('defines every stable repository control and semantic navigation location', () => {
    const controls = repositorySettingsControls('repo-1');
    expect(controls).toEqual([
      {
        id: 'repositories.repo-1.enabled_override',
        location: { section: 'repositories', path: ['repo-1', 'enablement', 'enabled_override'] },
      },
      {
        id: 'repositories.repo-1.pending_ci_mode_override',
        location: {
          section: 'repositories',
          path: ['repo-1', 'merge', 'pending_ci_mode_override'],
        },
      },
      {
        id: 'repositories.repo-1.pending_ci_branch_patterns_override.include',
        location: {
          section: 'repositories',
          path: ['repo-1', 'merge', 'pending_ci_branch_patterns_override', 'include'],
        },
      },
      {
        id: 'repositories.repo-1.pending_ci_branch_patterns_override.exclude',
        location: {
          section: 'repositories',
          path: ['repo-1', 'merge', 'pending_ci_branch_patterns_override', 'exclude'],
        },
      },
      {
        id: 'repositories.repo-1.pending_ci_quiet_period_seconds_override',
        location: {
          section: 'repositories',
          path: ['repo-1', 'merge', 'pending_ci_quiet_period_seconds_override'],
        },
      },
      {
        id: 'repositories.repo-1.path_index_interval_seconds_override',
        location: {
          section: 'repositories',
          path: ['repo-1', 'merge', 'path_index_interval_seconds_override'],
        },
      },
      {
        id: 'repositories.repo-1.ignore_repository_file',
        location: {
          section: 'repositories',
          path: ['repo-1', 'file', 'ignore_repository_file'],
        },
      },
      ...CONFIG_KEYS.map((key) => ({
        id: `repositories.repo-1.config_patch.${key}`,
        location: {
          section: 'repositories',
          path: [
            'repo-1',
            key === 'command_prefix' || key === 'allowed_commands' || key === 'command_aliases'
              ? 'commands'
              : 'behavior',
            key,
          ],
        },
      })),
    ]);
    expect(new Set(controls.map(({ id }) => id)).size).toBe(controls.length);
  });

  it('builds and overlays only complete editable state with deep clones', () => {
    const source = repository();
    const document = buildRepositorySettingsDocument(source);
    document.pending_ci_branch_patterns_override?.include.push('refs/heads/release/*');
    document.config_patch.allowed_commands?.push('merge');
    document.config_patch.command_aliases!.land = 'squash';

    expect(source.pending_ci_branch_patterns_override?.include).toEqual([]);
    expect(source.config_patch.allowed_commands).toEqual([]);
    expect(source.config_patch.command_aliases).toEqual({ ship: 'merge' });

    const overlay = overlayRepositorySettingsDocument(source, {
      ...document,
      enabled_override: true,
      pending_ci_mode_override: 'checks',
    });
    overlay.pending_ci_branch_patterns_override?.exclude.push('refs/heads/old/*');
    overlay.config_patch.allowed_commands?.push('approve');

    expect(overlay.repository.enabled_override).toBe(true);
    expect(overlay.repository.effective_enabled).toBe(false);
    expect(overlay.pending_ci_mode_inherited).toBe('checks');
    expect(overlay.revision).toBe(7);
    expect(source.repository.enabled_override).toBe(false);
    expect(document.pending_ci_branch_patterns_override?.exclude).toEqual([]);
    expect(document.config_patch.allowed_commands).toEqual(['merge']);
  });

  it('accepts exact complete documents and rejects partial, extended, or invalid state', () => {
    const valid = buildRepositorySettingsDocument(repository());
    expect(parseRepositorySettingsDocument(valid)).toEqual(valid);

    for (const value of [
      null,
      {},
      { ...valid, extra: true },
      { ...valid, enabled_override: 'false' },
      { ...valid, pending_ci_mode_override: 'statuses' },
      { ...valid, pending_ci_branch_patterns_override: { include: [], exclude: [], extra: [] } },
      { ...valid, pending_ci_branch_patterns_override: { include: [1], exclude: [] } },
      { ...valid, pending_ci_quiet_period_seconds_override: 86_401 },
      { ...valid, pending_ci_quiet_period_seconds_override: Number.NaN },
      { ...valid, path_index_interval_seconds_override: 604_801 },
      { ...valid, path_index_interval_seconds_override: 1.5 },
      { ...valid, config_patch: { unknown: true } },
      { ...valid, config_patch: { quiet_success: 'false' } },
      { ...valid, config_patch: { allowed_commands: ['merge', 1] } },
      { ...valid, config_patch: { command_aliases: { ship: false } } },
      { ...valid, ignore_repository_file: null },
    ]) {
      expect(parseRepositorySettingsDocument(value)).toBeNull();
    }
  });

  it('projects inheritance separately from explicit false and empty values', () => {
    const explicit = buildRepositorySettingsDocument(repository());
    const controls = repositorySettingsSavedControls('repo-1', explicit);

    expect(controls['repositories.repo-1.enabled_override']).toBe(false);
    expect(controls['repositories.repo-1.pending_ci_branch_patterns_override.include']).toEqual([]);
    expect(controls['repositories.repo-1.config_patch.quiet_success']).toEqual({
      overridden: true,
      value: false,
    });
    expect(controls['repositories.repo-1.config_patch.allowed_commands']).toEqual({
      overridden: true,
      value: [],
    });
    expect(controls['repositories.repo-1.config_patch.quiet_reactions']).toEqual({
      overridden: false,
      value: null,
    });

    explicit.enabled_override = null;
    explicit.pending_ci_branch_patterns_override = null;
    const inherited = repositorySettingsSavedControls('repo-1', explicit);
    expect(inherited['repositories.repo-1.enabled_override']).toBeNull();
    expect(inherited['repositories.repo-1.pending_ci_branch_patterns_override.include']).toBeNull();
    expect(inherited['repositories.repo-1.pending_ci_branch_patterns_override.exclude']).toBeNull();
  });

  it('serializes a batch input and commits the compact canonical response', () => {
    const document = buildRepositorySettingsDocument(repository());
    const input = repositorySettingsBatchInput('repo-1', 7, document);
    expect(input).toEqual({ repository_id: 'repo-1', ...document, expected_revision: 7 });

    const state: InstallationRepositorySettingsState = {
      ...input,
      revision: 12,
    };
    delete (state as Partial<typeof input>).expected_revision;
    const committed = repositorySettingsCommittedResource('target-1', state);
    expect(committed.resource).toEqual({
      type: 'repository-settings',
      targetId: 'target-1',
      repositoryId: 'repo-1',
    });
    expect(committed.revision).toBe(12);
    expect(committed.value).toEqual(document);
    expect(committed.savedControls).toEqual(repositorySettingsSavedControls('repo-1', document));
  });

  it('stages a complete document under one identified control and discards cleanly', () => {
    const source = repository();
    const drafts = new SettingsDraftRegistry({ storage: null, now: () => 1, writerId: 'test' });
    drafts.hydrate('viewer-1');
    expect(adoptRepositorySettings(drafts, 'target-1', source)).toBe(true);

    const next = {
      ...repositorySettingsDraftDocument(drafts, 'target-1', source),
      enabled_override: true,
    };
    expect(
      stageRepositorySettingsControl(
        drafts,
        'target-1',
        source,
        next,
        'repositories.repo-1.enabled_override',
      ),
    ).toBe(true);
    expect(drafts.dirtyControls()).toMatchObject([
      { id: 'repositories.repo-1.enabled_override', saved: false, value: true },
    ]);
    expect(repositorySettingsDraftDocument(drafts, 'target-1', source).enabled_override).toBe(true);

    expect(drafts.discardResource(repositorySettingsResource('target-1', 'repo-1'))).toBe(true);
    expect(repositorySettingsDraftDocument(drafts, 'target-1', source)).toEqual(
      buildRepositorySettingsDocument(source),
    );
  });
});

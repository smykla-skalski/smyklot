import { describe, expect, it } from 'vitest';

import { CONFIG_KEYS } from '../src/lib/config';
import { SettingsDraftRegistry } from '../src/lib/settings-drafts.svelte';
import {
  TARGET_DEFAULTS_CONTROLS,
  adoptTargetDefaults,
  buildTargetDefaultsDocument,
  overlayTargetDefaultsDocument,
  parseTargetDefaultsDocument,
  stageTargetDefaultsControl,
  targetDefaultsCommittedResource,
  targetDefaultsDraftDocument,
  targetDefaultsResource,
  targetDefaultsSavedControls,
} from '../src/lib/target-defaults-settings';
import type { ConfigSources, ConfigValues, PanelTarget } from '../src/lib/types';

const INHERITED: ConfigValues = {
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

const SOURCES: ConfigSources = {
  quiet_success: 'process',
  quiet_reactions: 'process',
  quiet_pending: 'process',
  allowed_commands: 'process',
  command_aliases: 'process',
  command_prefix: 'process',
  disable_mentions: 'process',
  disable_bare_commands: 'process',
  disable_unapprove: 'process',
  disable_reactions: 'process',
  disable_deleted_comments: 'process',
  allow_self_approval: 'process',
  allow_draft_merges: 'process',
};

function target(): PanelTarget {
  return {
    id: 'target-1',
    installation_id: 'installation-1',
    type: 'Organization',
    account: {
      id: 'account-1',
      provider: 'github:https://api.github.com',
      subject_id: 'subject-1',
      login: 'example',
      display_name: 'Example',
      avatar_url: null,
    },
    repository_default_enabled: false,
    pending_ci_mode_default: 'checks',
    pending_ci_branch_patterns_default: {
      include: ['~DEFAULT_BRANCH', 'refs/heads/release/*'],
      exclude: ['refs/heads/release/old'],
    },
    pending_ci_quiet_period_seconds_override: 45,
    pending_ci_quiet_period_seconds_inherited: 30,
    path_index_interval_seconds_override: 3_600,
    path_index_interval_seconds_inherited: 900,
    pending_ci_permissions: {
      checks_write: true,
      administration_write: true,
      merge_queues_read: true,
      commit_statuses_read: true,
    },
    config_patch: {
      quiet_success: true,
      quiet_reactions: false,
      quiet_pending: true,
      allowed_commands: ['approve', 'merge'],
      command_aliases: { ship: 'merge' },
      command_prefix: '/smyklot ',
      disable_mentions: true,
      disable_bare_commands: false,
      disable_unapprove: true,
      disable_reactions: false,
      disable_deleted_comments: true,
      allow_self_approval: false,
    },
    inherited_config: { ...INHERITED },
    effective_config: { ...INHERITED },
    config_sources: { ...SOURCES },
    revision: 7,
    repository_counts: { total: 3, enabled: 1, disabled: 2 },
    effective_role: 'owner',
    access_source: 'owner',
    capabilities: { read: true, write: true, manage_target_users: true },
  };
}

const EXPECTED_CONTROLS = [
  [
    'defaults.repository_default_enabled',
    { section: 'defaults', path: ['repositories', 'repository_default_enabled'] },
  ],
  [
    'defaults.path_index_interval_seconds_override',
    {
      section: 'defaults',
      path: ['repositories', 'path_index_interval_seconds_override'],
    },
  ],
  [
    'defaults.pending_ci_mode_default',
    { section: 'defaults', path: ['merge', 'pending_ci_mode_default'] },
  ],
  [
    'defaults.pending_ci_branch_patterns_default.include',
    {
      section: 'defaults',
      path: ['merge', 'pending_ci_branch_patterns_default', 'include'],
    },
  ],
  [
    'defaults.pending_ci_branch_patterns_default.exclude',
    {
      section: 'defaults',
      path: ['merge', 'pending_ci_branch_patterns_default', 'exclude'],
    },
  ],
  [
    'defaults.pending_ci_quiet_period_seconds_override',
    {
      section: 'defaults',
      path: ['merge', 'pending_ci_quiet_period_seconds_override'],
    },
  ],
  ...CONFIG_KEYS.map((key) => [
    `defaults.config_patch.${key}`,
    {
      section: 'defaults',
      path: [
        key === 'command_prefix' || key === 'allowed_commands' || key === 'command_aliases'
          ? 'commands'
          : 'behavior',
        key,
      ],
    },
  ]),
];

describe('target defaults settings adapter [Unit]', () => {
  it('defines every stable control ID and semantic navigation location', () => {
    expect(TARGET_DEFAULTS_CONTROLS).toEqual(
      EXPECTED_CONTROLS.map(([id, location]) => ({ id, location })),
    );
    expect(new Set(TARGET_DEFAULTS_CONTROLS.map(({ id }) => id)).size).toBe(
      TARGET_DEFAULTS_CONTROLS.length,
    );
  });

  it('builds one complete document without revision or inherited display state', () => {
    const source = target();

    expect(buildTargetDefaultsDocument(source)).toEqual({
      repository_default_enabled: false,
      pending_ci_mode_default: 'checks',
      pending_ci_branch_patterns_default: {
        include: ['~DEFAULT_BRANCH', 'refs/heads/release/*'],
        exclude: ['refs/heads/release/old'],
      },
      pending_ci_quiet_period_seconds_override: 45,
      path_index_interval_seconds_override: 3_600,
      config_patch: source.config_patch,
    });
  });

  it('clones nested document and overlay values without mutating either side', () => {
    const source = target();
    const document = buildTargetDefaultsDocument(source);
    document.pending_ci_branch_patterns_default.include.push('refs/heads/hotfix/*');
    document.config_patch.allowed_commands?.push('squash');
    if (document.config_patch.command_aliases !== undefined) {
      document.config_patch.command_aliases.land = 'squash';
    }

    expect(source.pending_ci_branch_patterns_default.include).toEqual([
      '~DEFAULT_BRANCH',
      'refs/heads/release/*',
    ]);
    expect(source.config_patch.allowed_commands).toEqual(['approve', 'merge']);
    expect(source.config_patch.command_aliases).toEqual({ ship: 'merge' });

    const overlay = overlayTargetDefaultsDocument(source, {
      ...document,
      repository_default_enabled: true,
      pending_ci_mode_default: 'labels',
    });
    overlay.pending_ci_branch_patterns_default.exclude.push('refs/heads/temporary/*');
    if (overlay.config_patch.command_aliases !== undefined) {
      overlay.config_patch.command_aliases.deploy = 'merge';
    }

    expect(overlay.repository_default_enabled).toBe(true);
    expect(overlay.pending_ci_mode_default).toBe('labels');
    expect(overlay.revision).toBe(7);
    expect(source.repository_default_enabled).toBe(false);
    expect(document.pending_ci_branch_patterns_default.exclude).toEqual(['refs/heads/release/old']);
    expect(document.config_patch.command_aliases).toEqual({ ship: 'merge', land: 'squash' });
  });

  it('parses exact finite documents and rejects malformed stored state', () => {
    const valid = buildTargetDefaultsDocument(target());
    expect(parseTargetDefaultsDocument(valid)).toEqual(valid);

    const invalid: unknown[] = [
      null,
      {},
      { ...valid, extra: true },
      { ...valid, repository_default_enabled: 'yes' },
      { ...valid, pending_ci_mode_default: 'statuses' },
      {
        ...valid,
        pending_ci_branch_patterns_default: { include: [], exclude: [], extra: [] },
      },
      {
        ...valid,
        pending_ci_branch_patterns_default: { include: [42], exclude: [] },
      },
      { ...valid, pending_ci_quiet_period_seconds_override: Number.POSITIVE_INFINITY },
      { ...valid, pending_ci_quiet_period_seconds_override: 86_401 },
      { ...valid, path_index_interval_seconds_override: 1.5 },
      { ...valid, path_index_interval_seconds_override: 604_801 },
      { ...valid, config_patch: { unknown: true } },
      { ...valid, config_patch: { quiet_success: 'yes' } },
      { ...valid, config_patch: { allowed_commands: ['merge', 12] } },
      { ...valid, config_patch: { command_aliases: { ship: 12 } } },
    ];
    for (const value of invalid) expect(parseTargetDefaultsDocument(value)).toBeNull();

    expect(() =>
      buildTargetDefaultsDocument({
        ...target(),
        pending_ci_quiet_period_seconds_override: Number.NaN,
      }),
    ).toThrow(TypeError);
  });

  it('projects every control and distinguishes a config override from inheritance', () => {
    const source = target();
    source.config_patch = {
      quiet_success: false,
      allowed_commands: [],
      command_aliases: Object.fromEntries([['__proto__', 'merge']]),
    };
    const document = buildTargetDefaultsDocument(source);
    const controls = targetDefaultsSavedControls(document);

    expect(Object.keys(controls)).toEqual(TARGET_DEFAULTS_CONTROLS.map(({ id }) => id));
    expect(controls['defaults.repository_default_enabled']).toBe(false);
    expect(controls['defaults.pending_ci_branch_patterns_default.include']).toEqual([
      '~DEFAULT_BRANCH',
      'refs/heads/release/*',
    ]);
    expect(controls['defaults.config_patch.quiet_success']).toEqual({
      overridden: true,
      value: false,
    });
    expect(controls['defaults.config_patch.allowed_commands']).toEqual({
      overridden: true,
      value: [],
    });
    expect(controls['defaults.config_patch.command_aliases']).toEqual({
      overridden: true,
      value: Object.fromEntries([['__proto__', 'merge']]),
    });
    expect(controls['defaults.config_patch.quiet_reactions']).toEqual({
      overridden: false,
      value: null,
    });

    const includes = controls['defaults.pending_ci_branch_patterns_default.include'];
    if (Array.isArray(includes)) includes.push('refs/heads/mutated/*');
    expect(document.pending_ci_branch_patterns_default.include).not.toContain(
      'refs/heads/mutated/*',
    );
  });

  it('creates a committed resource from the returned target revision and canonical values', () => {
    const returned = target();
    returned.revision = 12;
    returned.repository_default_enabled = true;
    returned.config_patch = { command_prefix: '!' };

    const committed = targetDefaultsCommittedResource(returned);

    expect(committed.resource).toEqual({ type: 'target-defaults', targetId: 'target-1' });
    expect(committed.revision).toBe(12);
    expect(committed.value).toEqual(buildTargetDefaultsDocument(returned));
    expect(committed.savedControls).toEqual(
      targetDefaultsSavedControls(buildTargetDefaultsDocument(returned)),
    );
    expect(committed.savedControls['defaults.config_patch.command_prefix']).toEqual({
      overridden: true,
      value: '!',
    });
    expect(committed.savedControls['defaults.config_patch.allowed_commands']).toEqual({
      overridden: false,
      value: null,
    });
  });

  it('stages identified controls, overlays the draft, and discards back to the server base', () => {
    const source = target();
    const drafts = new SettingsDraftRegistry({ storage: null, now: () => 1, writerId: 'test' });
    drafts.hydrate('viewer-1');
    expect(adoptTargetDefaults(drafts, source)).toBe(true);

    const next = {
      ...targetDefaultsDraftDocument(drafts, source),
      repository_default_enabled: true,
    };
    expect(
      stageTargetDefaultsControl(drafts, source, next, 'defaults.repository_default_enabled'),
    ).toBe(true);

    const shown = overlayTargetDefaultsDocument(
      source,
      targetDefaultsDraftDocument(drafts, source),
    );
    expect(shown.repository_default_enabled).toBe(true);
    expect(shown.revision).toBe(source.revision);
    expect(drafts.dirtyControls()).toMatchObject([
      { id: 'defaults.repository_default_enabled', saved: false, value: true },
    ]);

    expect(drafts.discardResource(targetDefaultsResource(source.id))).toBe(true);
    expect(targetDefaultsDraftDocument(drafts, source)).toEqual(
      buildTargetDefaultsDocument(source),
    );
  });

  it('preserves a draft and marks a conflict when a newer canonical document arrives', () => {
    const source = target();
    const drafts = new SettingsDraftRegistry({ storage: null, now: () => 1, writerId: 'test' });
    drafts.hydrate('viewer-1');
    adoptTargetDefaults(drafts, source);

    const draft = {
      ...targetDefaultsDraftDocument(drafts, source),
      repository_default_enabled: true,
    };
    stageTargetDefaultsControl(drafts, source, draft, 'defaults.repository_default_enabled');

    const concurrent = {
      ...source,
      revision: source.revision + 1,
      pending_ci_quiet_period_seconds_override: 60,
    };
    expect(adoptTargetDefaults(drafts, concurrent)).toBe(false);
    expect(targetDefaultsDraftDocument(drafts, concurrent)).toEqual(draft);
    expect(drafts.resource(targetDefaultsResource(source.id))?.conflict).toMatchObject({
      type: 'revision',
      actualRevision: source.revision + 1,
    });
    expect(drafts.dirtyControls()).toHaveLength(1);
  });
});

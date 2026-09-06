import { describe, expect, it } from 'vitest';
import { mockBypassActorSuggestions } from '../dev/bypass-actors';
import { parseBypassPolicy } from '../src/lib/bypass-policy';
import { SettingsDraftRegistry } from '../src/lib/settings-drafts.svelte';
import {
  adoptTargetDefaults,
  buildTargetDefaultsDocument,
  parseTargetDefaultsDocument,
  overlayTargetDefaultsDocument,
  stageTargetDefaultsControl,
  targetDefaultsDraftDocument,
} from '../src/lib/target-defaults-settings';
import {
  buildRepositorySettingsDocument,
  parseRepositorySettingsDocument,
  repositorySettingsBatchInput,
} from '../src/lib/repository-settings';
import { TARGET, REPOSITORY_DETAIL } from '../stories/support/fixtures';

const APP = { actor_id: 1197525, actor_type: 'Integration', bypass_mode: 'always' };

describe('bypass policy documents [Unit]', () => {
  it('clears the draft when a removed actor is restored in a different list position', () => {
    const team = { actor_type: 'Team', actor_id: 42, bypass_mode: 'pull_request' };
    const target = {
      ...TARGET,
      pending_ci_bypass_policy_default: { allow: true, actors: [team, APP] },
    };
    const drafts = new SettingsDraftRegistry({ storage: null, now: () => 1, writerId: 'test' });
    drafts.hydrate('viewer');
    adoptTargetDefaults(drafts, target);
    const saved = buildTargetDefaultsDocument(target);
    for (const actors of [[team], [APP, team]]) {
      expect(
        stageTargetDefaultsControl(
          drafts,
          target,
          {
            ...saved,
            pending_ci_bypass_policy_default: { allow: true, actors },
          },
          'defaults.pending_ci_bypass_policy_default',
        ),
      ).toBe(true);
      expect(drafts.dirtyControls()).toHaveLength(actors.length === 1 ? 1 : 0);
    }
    expect(targetDefaultsDraftDocument(drafts, target)).toEqual(saved);
    expect(drafts.beginSave({ type: 'workspace', targetId: target.id })).toBeNull();
  });

  it('keeps rollout preservation distinct from a managed deny with retained actors', () => {
    expect(parseBypassPolicy(null)).toBeNull();
    expect(parseBypassPolicy({ allow: false, actors: [APP] })).toEqual({
      allow: false,
      actors: [APP],
    });
    const allowed = parseBypassPolicy({ allow: true, actors: [APP] });
    expect(allowed).toEqual({ allow: true, actors: [APP] });
    allowed!.actors[0]!.actor_id = 9;
    expect(APP.actor_id).toBe(1197525);
  });

  it.each([
    {},
    { allow: true },
    { allow: 'true', actors: [] },
    { allow: true, actors: [], extra: true },
    { allow: true, actors: [APP, APP] },
    { allow: true, actors: [{ ...APP, actor_id: 0 }] },
    { allow: true, actors: [{ ...APP, actor_id: Number.MAX_SAFE_INTEGER + 1 }] },
    { allow: true, actors: [{ ...APP, bypass_mode: 'sometimes' }] },
    { allow: true, actors: [{ ...APP, actor_type: 'User', actor_id: -1 }] },
    { allow: true, actors: [{ ...APP, actor_type: 'DeployKey', bypass_mode: 'pull_request' }] },
    { allow: true, actors: [{ ...APP, name: 'untrusted identity' }] },
  ])('rejects malformed or ambiguous policy %#', (value) => {
    expect(parseBypassPolicy(value)).toBeUndefined();
  });

  it('reads drafts from before the new field without granting an exception', () => {
    const target: Record<string, unknown> = buildTargetDefaultsDocument(TARGET);
    const repository: Record<string, unknown> = buildRepositorySettingsDocument(REPOSITORY_DETAIL);
    delete target.pending_ci_bypass_policy_default;
    delete repository.pending_ci_bypass_policy_override;
    expect(parseTargetDefaultsDocument(target)?.pending_ci_bypass_policy_default).toBeNull();
    expect(
      parseRepositorySettingsDocument(repository)?.pending_ci_bypass_policy_override,
    ).toBeNull();
  });

  it('carries managed exceptions through target drafts and repository saves', () => {
    const policy = { allow: true, actors: [APP] };
    const document = buildTargetDefaultsDocument({
      ...TARGET,
      pending_ci_bypass_policy_default: policy,
    });
    expect(
      overlayTargetDefaultsDocument(TARGET, document).pending_ci_bypass_policy_default,
    ).toEqual(policy);
    const repository = buildRepositorySettingsDocument({
      ...REPOSITORY_DETAIL,
      pending_ci_bypass_policy_override: policy,
    });
    expect(
      repositorySettingsBatchInput(REPOSITORY_DETAIL.repository.id, 3, repository)
        .pending_ci_bypass_policy_override,
    ).toEqual(policy);
    const inherited = parseRepositorySettingsDocument({
      ...repository,
      pending_ci_bypass_policy_override: null,
    });
    expect(inherited?.pending_ci_bypass_policy_override).toBeNull();
    expect(
      parseTargetDefaultsDocument({
        ...document,
        pending_ci_bypass_policy_default: { allow: true },
      }),
    ).toBeNull();
  });
});

describe('bypass actor mock suggestions [Unit]', () => {
  it('covers app installation states with named suggestions', () => {
    const apps = mockBypassActorSuggestions('Integration', '');
    expect(apps.map((app) => app.installation_status)).toEqual([
      'all_repositories',
      'selected_repositories',
      'not_installed',
      'suspended',
    ]);
    expect(mockBypassActorSuggestions('Integration', 'NOV')).toMatchObject([
      { name: 'Renovate', actor_id: 2740 },
    ]);
    expect(mockBypassActorSuggestions('User', 'the octocat')).toMatchObject([{ slug: 'octocat' }]);
    expect(mockBypassActorSuggestions('Integration', 'no-such-app')).toEqual([]);
  });
});

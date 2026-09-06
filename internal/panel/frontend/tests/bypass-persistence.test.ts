import { describe, expect, it } from 'vitest';
import { createPanelApi } from '../src/lib/api';
import { parseBypassPolicy } from '../src/lib/bypass-policy';
import { parseJson, preserveNumberToken } from '../src/lib/merge';
import {
  adoptRepositorySettings,
  buildRepositorySettingsDocument,
  repositorySettingsDraftDocument,
  stageRepositorySettingsControl,
} from '../src/lib/repository-settings';
import { SettingsDraftRegistry } from '../src/lib/settings-drafts.svelte';
import {
  adoptTargetDefaults,
  buildTargetDefaultsDocument,
  stageTargetDefaultsControl,
  targetDefaultsDraftDocument,
} from '../src/lib/target-defaults-settings';
import { saveWorkspaceDrafts } from '../src/lib/workspace-settings-save';
import { TARGET, REPOSITORY_DETAIL } from '../stories/support/fixtures';

const policy = parseBypassPolicy(
  parseJson(
    '{"allow":true,"actors":[{"actor_id":9007199254740992,"actor_type":"Integration","bypass_mode":"always"},{"actor_id":9007199254740993,"actor_type":"Integration","bypass_mode":"always"},{"actor_id":9223372036854775807,"actor_type":"RepositoryRole","bypass_mode":"always"}]}',
  ),
)!;
const actorIds = (value: typeof policy) =>
  JSON.stringify(value.actors.map((actor) => actor.actor_id));
const exactIds = '[9007199254740992,9007199254740993,9223372036854775807]';

describe('bypass identity persistence [Unit]', () => {
  it('reads exact directory and settings IDs while keeping quantity metadata numeric', async () => {
    const responses = [
      {
        items: policy.actors.map((actor) => ({
          ...actor,
          name: 'Release actor',
          slug: 'release',
          avatar_url: null,
        })),
      },
      { targets: [{ ...TARGET, pending_ci_bypass_policy_default: policy }] },
      { ...REPOSITORY_DETAIL, pending_ci_bypass_policy_override: policy },
    ];
    const api = createPanelApi('', async () => new Response(JSON.stringify(responses.shift())));
    const directory = await api.fetchBypassActors(TARGET.id);
    expect(JSON.stringify(directory.items.map((actor) => actor.actor_id))).toBe(exactIds);
    const target = (await api.fetchTargets())[0]!;
    expect(actorIds(buildTargetDefaultsDocument(target).pending_ci_bypass_policy_default!)).toBe(
      exactIds,
    );
    expect(target.revision).toBe(TARGET.revision);
    expect(typeof target.repository_default_enabled).toBe('boolean');
    const repository = await api.fetchRepository(TARGET.id, REPOSITORY_DETAIL.repository.id);
    expect(
      actorIds(buildRepositorySettingsDocument(repository).pending_ci_bypass_policy_override!),
    ).toBe(exactIds);
    expect(repository.revision).toBe(REPOSITORY_DETAIL.revision);
  });

  it('reloads drafts, clears exact reverts, and sends int64 policies in one numeric JSON batch', async () => {
    const target = { ...TARGET, pending_ci_bypass_policy_default: policy };
    const repository = { ...REPOSITORY_DETAIL, pending_ci_bypass_policy_override: policy };
    const records = new Map<string, string>();
    const storage = {
      getItem: (key: string) => records.get(key) ?? null,
      setItem: (key: string, value: string) => {
        records.set(key, value);
      },
    };
    const registry = (writerId: string) => {
      const drafts = new SettingsDraftRegistry({ storage, now: () => 1, writerId });
      drafts.hydrate('viewer');
      adoptTargetDefaults(drafts, target);
      adoptRepositorySettings(drafts, target.id, repository);
      return drafts;
    };
    const stage = (drafts: SettingsDraftRegistry, value: typeof policy) => {
      expect(
        stageTargetDefaultsControl(
          drafts,
          target,
          {
            ...buildTargetDefaultsDocument(target),
            pending_ci_bypass_policy_default: value,
          },
          'defaults.pending_ci_bypass_policy_default',
        ),
      ).toBe(true);
      expect(
        stageRepositorySettingsControl(
          drafts,
          target.id,
          repository,
          {
            ...buildRepositorySettingsDocument(repository),
            pending_ci_bypass_policy_override: value,
          },
          `repositories.${repository.repository.id}.pending_ci_bypass_policy_override`,
        ),
      ).toBe(true);
    };
    const next = {
      ...policy,
      actors: policy.actors.map((actor, index) =>
        index === 1 ? { ...actor, bypass_mode: 'pull_request' } : actor,
      ),
    };
    const first = registry('first');
    stage(first, next);
    expect(first.dirtyControlCount).toBe(2);
    const reopened = registry('reopened');
    expect(
      actorIds(targetDefaultsDraftDocument(reopened, target).pending_ci_bypass_policy_default!),
    ).toBe(exactIds);
    expect(
      actorIds(
        repositorySettingsDraftDocument(reopened, target.id, repository)
          .pending_ci_bypass_policy_override!,
      ),
    ).toBe(exactIds);
    stage(reopened, { ...policy, actors: [...policy.actors].reverse() });
    expect(reopened.dirtyControlCount).toBe(0);
    stage(reopened, next);
    const api = createPanelApi('', async (_url, init) => {
      const body = String(init?.body);
      expect(body.match(/"actor_id":9007199254740993/gu)).toHaveLength(2);
      expect(body.match(/"actor_id":9223372036854775807/gu)).toHaveLength(2);
      const input = JSON.parse(body, preserveNumberToken);
      return new Response(
        JSON.stringify({
          target: { ...input.target, target_id: target.id, revision: target.revision + 1 },
          repositories: [{ ...input.repositories[0], revision: repository.revision + 1 }],
        }),
      );
    });
    expect((await saveWorkspaceDrafts(reopened, target.id, api.saveWorkspaceSettings)).saved).toBe(
      true,
    );
    expect(reopened.dirtyControlCount).toBe(0);
    expect(
      actorIds(targetDefaultsDraftDocument(reopened, target).pending_ci_bypass_policy_default!),
    ).toBe(exactIds);
    expect(
      actorIds(
        repositorySettingsDraftDocument(reopened, target.id, repository)
          .pending_ci_bypass_policy_override!,
      ),
    ).toBe(exactIds);
  });
});

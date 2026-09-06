import type { BypassActorIdentity, SyncRulesetBypassActor } from '../src/lib/types.js';

// Stored exceptions whose directory entries are withheld in recovery scenarios
// The second team/person and both custom roles are synthetic fixture identities
export const MOCK_UNRESOLVED_BYPASS_ACTORS: SyncRulesetBypassActor[] = [
  { actor_type: 'Integration', actor_id: 2740, bypass_mode: 'always' },
  { actor_type: 'Integration', actor_id: 254, bypass_mode: 'pull_request' },
  { actor_type: 'Team', actor_id: 64120, bypass_mode: 'always' },
  { actor_type: 'Team', actor_id: 64121, bypass_mode: 'pull_request' },
  { actor_type: 'User', actor_id: 583231, bypass_mode: 'always' },
  { actor_type: 'User', actor_id: 583232, bypass_mode: 'pull_request' },
  { actor_type: 'RepositoryRole', actor_id: 901, bypass_mode: 'always' },
  { actor_type: 'RepositoryRole', actor_id: 902, bypass_mode: 'pull_request' },
];

// App IDs, names, and logos come from GitHub's public app metadata
// Installation states are simulated to exercise every permission state in the mock
export const MOCK_BYPASS_ACTORS: BypassActorIdentity[] = [
  {
    actor_id: 1197525,
    actor_type: 'Integration',
    name: 'smyklot',
    slug: 'smyklot',
    installation_status: 'all_repositories',
    avatar_url: 'https://avatars.githubusercontent.com/in/1197525?v=4',
  },
  {
    actor_id: 2740,
    actor_type: 'Integration',
    name: 'Renovate',
    slug: 'renovate',
    installation_status: 'selected_repositories',
    avatar_url:
      'https://avatars.githubusercontent.com/in/2740?u=faebe63c0f2a1bc3005b90a70d1fb9c1e14e196a&v=4',
  },
  {
    actor_id: 29110,
    actor_type: 'Integration',
    name: 'Dependabot',
    slug: 'dependabot',
    installation_status: 'not_installed',
    avatar_url:
      'https://avatars.githubusercontent.com/in/29110?u=666c3b834dda4100589c03841fb2abb581ba5891&v=4',
  },
  {
    actor_id: 254,
    actor_type: 'Integration',
    name: 'Codecov',
    slug: 'codecov',
    installation_status: 'suspended',
    avatar_url:
      'https://avatars.githubusercontent.com/in/254?u=c392dd7c2afc51fe2347d4296afa8494ac05772e&v=4',
  },
  {
    actor_id: 64120,
    actor_type: 'Team',
    name: 'Release engineering',
    slug: 'release-engineering',
    avatar_url: null,
  },
  {
    actor_id: 583231,
    actor_type: 'User',
    name: 'The Octocat',
    slug: 'octocat',
    avatar_url: 'https://avatars.githubusercontent.com/u/583231?v=4',
  },
];

export function mockBypassActorSuggestions(
  type: string | null,
  query: string,
): BypassActorIdentity[] {
  const needle = query.trim().toLocaleLowerCase();
  return MOCK_BYPASS_ACTORS.filter(
    (item) =>
      (!type || item.actor_type === type) &&
      (!needle || `${item.name} ${item.slug}`.toLocaleLowerCase().includes(needle)),
  );
}

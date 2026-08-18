/**
 * A `PanelApi` that answers nothing.
 *
 * Stories seed the query cache or pass data as props; what they still need is an
 * object of the right shape to hand components that take `api`. Every method rejects
 * rather than resolving empty, because a story that accidentally depends on a request
 * should fail loudly in the catalogue rather than quietly render an empty state that
 * looks deliberate.
 *
 * The one exception is `signInUrl`, which is read synchronously to build an href.
 */
import type { PanelApi } from '#lib/api.js';

import {
  AUDIT,
  FAILURES,
  INSTALLATIONS,
  INVITATIONS,
  NOTIFICATIONS,
  OVERVIEW,
  REPOSITORIES,
  REPOSITORY_DETAIL,
  TARGET,
  USERS,
} from './fixtures.js';

function refuse(name: string) {
  return () =>
    Promise.reject(
      new Error(`${name} was called from a story - pass the data in, or seed the query cache`),
    );
}

export function stubApi(over: Partial<PanelApi> = {}): PanelApi {
  return new Proxy(
    {
      signInUrl: () => 'https://github.com/login/oauth/authorize',
      ...over,
    } as PanelApi,
    {
      get(target, property) {
        if (property in target) return target[property as keyof PanelApi];
        /*
         * Answer only what a `PanelApi` could be asked for. A trap that refuses
         * *everything* also answers the protocol lookups the runtime makes on any
         * object it handles: `JSON.stringify` finds a `toJSON` function and gets a
         * rejected promise nothing is waiting on, which is what put 31 unhandled
         * rejections into the catalogue when Storybook serialised story args. `then`
         * is worse - answering it makes this thenable, so `await stubApi()` never
         * settles.
         */
        if (typeof property !== 'string') return undefined;
        if (property === 'toJSON' || property === 'then') return undefined;
        return refuse(property);
      },
    },
  );
}

/**
 * A `PanelApi` that answers from the mock's own fixtures.
 *
 * `stubApi` is right for a component that takes `api` as a prop: the story hands in
 * exactly what that component reads, and anything else failing loudly is the point.
 * It is wrong for a component that reads `session.api`, because there the story has no
 * say in which methods get called - `InstallationView` alone reaches twenty of them,
 * and refusing all twenty draws a shell over nothing.
 *
 * So the reads answer, out of `dev/fixtures.ts` - the same data the dev server serves,
 * which is what stops the catalogue and the running app disagreeing. The writes still
 * refuse, and that is deliberate: a story is a picture of a state, and one that let a
 * mutation "succeed" against a fixture would show a result no service produced.
 *
 * Anything not listed refuses by name through `stubApi`, so the next method a view
 * starts calling says so instead of silently resolving undefined.
 */
export function fixtureApi(over: Partial<PanelApi> = {}): PanelApi {
  const page = <T>(items: readonly T[]) => ({
    items: [...items],
    next_cursor: null,
    total: items.length,
  });

  return stubApi({
    fetchTargets: async () => [TARGET],
    fetchRepositories: async () => page(REPOSITORIES),
    fetchRepository: async () => REPOSITORY_DETAIL,
    fetchAudit: async () => page(AUDIT),
    fetchFailures: async () => page(FAILURES),
    fetchTargetUsers: async () => page(USERS),
    fetchTargetInvitations: async () => page(INVITATIONS),
    fetchUserDecisions: async () => [],
    suggestUsers: async () => [],
    fetchRootOverview: async () => OVERVIEW,
    fetchRootInstallations: async () => INSTALLATIONS,
    fetchNotifications: async () => NOTIFICATIONS,
    ...over,
  } as Partial<PanelApi>);
}

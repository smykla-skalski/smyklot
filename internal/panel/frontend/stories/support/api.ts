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
      get(target, property: string) {
        if (property in target) return target[property as keyof PanelApi];
        return refuse(property);
      },
    },
  );
}

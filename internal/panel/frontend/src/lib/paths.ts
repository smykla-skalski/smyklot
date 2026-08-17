import { resolve } from '$app/paths';

import { normalizeBasePath } from './base.ts';

/**
 * The path the panel is served under: `''`, or `/something` with no trailing slash.
 *
 * SvelteKit 3 removed `base` from `$app/paths`, and `resolve('')` is the one place the
 * value is still reachable - as `base + '/'`, because `resolve` joins its argument to
 * the base with a separator. So the separator comes straight back off. Every caller
 * spells the rest itself, as `${basePath}/inbox` or through `panelUrl`, and a base that
 * ended in a slash would give each of them two.
 *
 * Read at module scope on purpose: `paths.base` is fixed at build time, and the Go
 * server rewrites the sentinel it holds across the whole bundle before anything runs.
 *
 * Its own module rather than a line in `base.ts`, because that one is loaded by the
 * mock server and the route manifest with plain Node, which knows nothing of
 * SvelteKit's aliases and cannot resolve `$app/paths`.
 */
export const basePath: string = normalizeBasePath(resolve(''));

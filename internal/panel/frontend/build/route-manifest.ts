import { writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

import type { Adapter, Builder } from '@sveltejs/kit';

/**
 * Writes the router's own route table into the built bundle, for the Go server.
 *
 * The server decides whether an address gets the application shell or the not-found
 * page, and it decides a request before any of this code runs. It used to decide it
 * from a copy of the route tree written by hand in Go, and a copy drifts: the queue
 * shipped three addresses that the panel linked to, navigated to and rendered, and
 * that a reload answered with 404, because nobody thought to add them twice.
 *
 * So the copy is generated. `builder.routes` is SvelteKit's own parse of
 * `src/routes` - the same one the client router is built from - and `route.pattern`
 * is the very regular expression it matches URLs with. Both cross into the manifest
 * unaltered. What does not cross by itself is the matchers, because those are code;
 * `src/params.ts` declares each one's `pattern`, and this reads them from the very
 * module the router loads.
 *
 * Nothing here decides anything about routing. If this file has to be edited to add
 * a route, it is wrong.
 */

/** Parameters as they are spelled in a route id: `[a]`, `[a=m]`, `[[a=m]]`, `[...a]`, `[...a=m]`. */
const PARAMETER = /\[{1,2}(?:\.{3})?([A-Za-z0-9_$]+)(?:=([A-Za-z0-9_$]+))?\]{1,2}/gu;

export interface RouteManifest {
  version: number;
  routes: Array<{
    id: string;
    pattern: string;
    params: Array<{ name: string; matcher: string | null }>;
  }>;
}

/**
 * Counts the capturing groups in a pattern by making every one of them optional.
 *
 * `exec` on a pattern that can match nothing reports one slot per group whether or
 * not it participated, so the arity comes back without the pattern having to match
 * anything real.
 */
function capturingGroups(source: string): number {
  const probe = new RegExp(`${source}|`).exec('');

  return (probe?.length ?? 1) - 1;
}

/**
 * Constructs RE2 has no answer for, so the Go server could not compile a pattern
 * carrying one. RE2 refuses them by design rather than by omission - each needs
 * backtracking - so a pattern that grows one is a change of routing vocabulary, and
 * it has to stop the build here rather than the service at its next start.
 */
const UNSUPPORTED: Array<[RegExp, string]> = [
  [/\(\?=/u, 'lookahead'],
  [/\(\?!/u, 'negative lookahead'],
  [/\(\?<=/u, 'lookbehind'],
  [/\(\?<!/u, 'negative lookbehind'],
  [/\\[1-9]/u, 'backreference'],
];

/**
 * Rewrites a JavaScript route pattern into the same expression in RE2.
 *
 * SvelteKit spells "any character, newline included" as the empty negated class
 * `[^]`, which JavaScript accepts and RE2 rejects outright - it is the one dialect
 * difference in the vocabulary the router generates, and it appears in every rest
 * parameter. `[\s\S]` says the same thing to both. The substring is unambiguous: a
 * class with contents cannot contain it, since `[^abc]` reads `[^a` at that offset.
 */
function toRE2(source: string, id: string): string {
  const translated = source.replaceAll('[^]', '[\\s\\S]');
  for (const [construct, name] of UNSUPPORTED) {
    if (construct.test(translated)) {
      throw new Error(
        `route ${id} compiles to a pattern using ${name}, which RE2 cannot express, ` +
          'so the Go server could not answer this address',
      );
    }
  }

  return translated;
}

function parametersOf(id: string): Array<{ name: string; matcher: string | null }> {
  return [...id.matchAll(PARAMETER)].map((match) => ({
    name: match[1] ?? '',
    matcher: match[2] ?? null,
  }));
}

/**
 * Reads the matchers' declared patterns from the module the router itself imports.
 *
 * Deliberately the module and not the file's text: a matcher missing from `patterns`
 * has to fail the build rather than parse as having none, which would quietly widen
 * the server to every value of that parameter.
 */
async function matcherPatterns(
  names: Iterable<string>,
  modulePath: string,
): Promise<Map<string, string>> {
  const { patterns } = (await import(pathToFileURL(modulePath).href)) as {
    patterns?: Record<string, unknown>;
  };
  if (typeof patterns !== 'object' || patterns === null) {
    throw new Error(
      `${modulePath} must export a \`patterns\` record, so the route manifest can hand ` +
        'the same rules to the Go server',
    );
  }

  const resolved = new Map<string, string>();
  for (const name of new Set(names)) {
    const pattern = patterns[name];
    if (typeof pattern !== 'string' || pattern === '') {
      throw new Error(
        `param matcher "${name}" must declare a non-empty pattern in \`patterns\`, so ` +
          'the route manifest can hand the same rule to the Go server',
      );
    }
    new RegExp(pattern); // Throws here rather than at the server's startup.
    resolved.set(name, toRE2(pattern, `matcher ${name}`));
  }

  return resolved;
}

export async function routeManifest(builder: Builder, paramsModule: string) {
  // A route without a page renders nothing, so the shell would be served for an
  // address that resolves to no document.
  const pages = builder.routes.filter((route) => route.page.methods.includes('GET'));
  if (pages.length === 0) {
    throw new Error('the route manifest found no page routes, which cannot be right');
  }

  const matcherNames = pages.flatMap((route) =>
    parametersOf(route.id)
      .map((parameter) => parameter.matcher)
      .filter((matcher): matcher is string => matcher !== null),
  );
  const patterns = await matcherPatterns(matcherNames, paramsModule);

  const routes = pages
    .map((route) => {
      const params = parametersOf(route.id);
      const groups = capturingGroups(route.pattern.source);
      // The manifest pairs each parameter with a capturing group by position, which
      // is how the router reads them too. If that ever stops holding, every matcher
      // would be checked against the wrong segment - so prove it here instead.
      if (groups !== params.length) {
        throw new Error(
          `route ${route.id} has ${params.length} parameters but ${groups} capturing ` +
            'groups, so the manifest cannot pair them',
        );
      }

      return {
        id: route.id,
        pattern: toRE2(route.pattern.source, route.id),
        params: params.map((parameter) => ({
          name: parameter.name,
          matcher: parameter.matcher === null ? null : (patterns.get(parameter.matcher) ?? null),
        })),
      };
    })
    .sort((left, right) => (left.id < right.id ? -1 : left.id > right.id ? 1 : 0));

  return { version: 1, routes } satisfies RouteManifest;
}

/**
 * Wraps an adapter so the build also writes `routes.json` beside its output.
 *
 * After the wrapped adapter, never before: it is what creates the directory, and on
 * a clean build it empties it first.
 */
export function withRouteManifest(
  inner: Adapter,
  options: { out: string; params: string },
): Adapter {
  return {
    ...inner,
    name: `${inner.name}+route-manifest`,
    async adapt(builder: Builder) {
      await inner.adapt(builder);
      const manifest = await routeManifest(builder, resolve(options.params));
      writeFileSync(
        join(resolve(options.out), 'routes.json'),
        `${JSON.stringify(manifest, null, 2)}\n`,
      );
      builder.log.minor(`wrote routes.json with ${manifest.routes.length} routes`);
    },
  };
}

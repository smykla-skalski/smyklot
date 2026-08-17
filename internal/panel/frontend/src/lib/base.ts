const VERSION_META_NAME = 'smyklot-panel-version';
const SERVICE_META_NAME = 'smyklot-panel-service';

/**
 * A placeholder the server has not filled in, recognised by its shape.
 *
 * Never by its value. The server substitutes each of its sentinels wherever one
 * appears as a complete string literal in a served text asset, and this module
 * is built into one: a constant holding the version sentinel comes back from
 * that rewrite holding the version instead, so the equality is true for the
 * real value and every build reports itself as unset. That is how the released
 * panel came to render no footer while the meta tag beside it carried the right
 * version all along.
 *
 * A pattern never spells a whole sentinel, so nothing rewrites it, and nothing
 * the server injects can match one - a version and a host both carry dots.
 * `tests/base.test.ts` holds the rule that keeps a sentinel out of this file.
 */
const PLACEHOLDER = /^__[a-z0-9_]+__$/u;

/**
 * The part of a document this module reads.
 *
 * Structural rather than `Document`, because this module has to load where the DOM
 * types do not exist: plain Node runs it for the route manifest, and the service
 * worker's TypeScript project compiles against `WebWorker` rather than `DOM`. Naming
 * `Document` here made both of those a type error for a pair of methods.
 */
export interface MetaSource {
  querySelector(selectors: string): { getAttribute(name: string): string | null } | null;
}

export interface PanelBuild {
  version: string | null;
  serviceHost: string | null;
}

export function readPanelBuild(source: MetaSource): PanelBuild {
  return {
    version: readInjected(source, VERSION_META_NAME),
    serviceHost: readInjected(source, SERVICE_META_NAME),
  };
}

function readInjected(source: MetaSource, name: string): string | null {
  const content = source.querySelector(`meta[name="${name}"]`)?.getAttribute('content')?.trim();
  if (content === undefined || content === '' || PLACEHOLDER.test(content)) {
    return null;
  }
  return content;
}

export function normalizeBasePath(raw: string): string {
  const trimmed = raw.trim().replace(/\/+$/, '');
  if (trimmed === '') {
    return '';
  }
  return trimmed.startsWith('/') ? trimmed : `/${trimmed}`;
}

export function panelUrl(base: string, path: string): string {
  const suffix = path.startsWith('/') ? path : `/${path}`;
  return `${normalizeBasePath(base)}${suffix}`;
}

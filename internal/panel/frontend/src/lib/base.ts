const BASE_META_NAME = 'smyklot-panel-base';
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

export interface PanelBuild {
  version: string | null;
  serviceHost: string | null;
}

export function readBasePath(source: Document): string {
  const meta = source.querySelector(`meta[name="${BASE_META_NAME}"]`);
  const content = meta?.getAttribute('content');
  if (content === null || content === undefined) {
    throw new Error(`the panel page is missing its <meta name="${BASE_META_NAME}"> element`);
  }
  return normalizeBasePath(content);
}

export function readPanelBuild(source: Document): PanelBuild {
  return {
    version: readInjected(source, VERSION_META_NAME),
    serviceHost: readInjected(source, SERVICE_META_NAME),
  };
}

function readInjected(source: Document, name: string): string | null {
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

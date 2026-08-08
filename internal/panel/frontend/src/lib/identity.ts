/**
 * Turning a stored account into the two things a reader recognises: a handle,
 * and the installation it came from when that is not the obvious one.
 */

/** The public GitHub API base, the installation that needs no explaining. */
const PUBLIC_GITHUB_HOST = 'api.github.com';

/** How an account is named on the page. */
export interface Handle {
  /** The login as people write it to each other. */
  handle: string;
  /**
   * The installation the account came from, or `null` when it is public GitHub.
   * Accounts are keyed by `provider:subject_id`, and the provider carries a
   * whole API base, which is a namespace rather than anything a reader wants.
   */
  host: string | null;
}

/**
 * Read an account's provider and login into something addressable.
 *
 * The stored provider looks like `github:https://api.github.com`, which on
 * screen said more about how the panel keys accounts than about who signed in.
 */
export function readHandle(provider: string, login: string): Handle {
  return { handle: `@${login}`, host: installationHost(provider) };
}

/**
 * The handle as one run of text, with the installation appended when there is
 * one.
 *
 * Joined here rather than in markup: a template that puts the separator inside a
 * conditional loses the space in front of it to Svelte's whitespace trimming,
 * and the result reads as `@ada· ghe.example.com`.
 */
export function handleLabel(handle: Handle): string {
  return handle.host === null ? handle.handle : `${handle.handle} · ${handle.host}`;
}

function installationHost(provider: string): string | null {
  if (provider === '') {
    return null;
  }
  const separator = provider.indexOf(':');
  const base = separator === -1 ? provider : provider.slice(separator + 1);
  let host: string;
  try {
    host = new URL(base).host;
  } catch {
    // An unrecognised provider shown verbatim is worse than a clean handle but
    // better than hiding which installation an account came from.
    return provider;
  }
  return host === PUBLIC_GITHUB_HOST ? null : host;
}

/**
 * The letters to show in place of an avatar the browser could not load.
 *
 * Split by grapheme rather than by code unit: cutting a surrogate pair in half
 * renders a replacement glyph exactly where the fallback is meant to be.
 */
export function monogram(displayName: string, login: string): string {
  const words = displayName.trim().split(/\s+/).filter(Boolean);
  if (words.length === 0) {
    const fallback = firstGrapheme(login.trim());
    return fallback === '' ? '?' : fallback.toLocaleUpperCase();
  }
  const first = firstGrapheme(words[0] ?? '');
  const last = words.length > 1 ? firstGrapheme(words[words.length - 1] ?? '') : '';
  return `${first}${last}`.toLocaleUpperCase();
}

/**
 * Built once and reused. Constructing a segmenter resolves a locale and builds an
 * ICU break iterator, which measures around fifty times the cost of the rest of
 * this module's work, and a monogram is recomputed on every roster render. Held
 * lazily so a runtime without `Intl.Segmenter` never tries.
 */
let graphemes: Intl.Segmenter | null = null;

function firstGrapheme(value: string): string {
  if (value === '') {
    return '';
  }
  if (typeof Intl.Segmenter === 'function') {
    graphemes ??= new Intl.Segmenter(undefined, { granularity: 'grapheme' });
    for (const { segment } of graphemes.segment(value)) {
      return segment;
    }
    return '';
  }
  return Array.from(value)[0] ?? '';
}

import { basePath } from '../../src/lib/paths.ts';

/**
 * An address the panel could actually be at.
 *
 * Carries the configured base, because SvelteKit resolves addresses now and adds it. A
 * fixture without one is an address the panel never sees, so a specification written
 * against it proves nothing about the mount the panel is served from.
 *
 * Shared rather than declared per file: two suites had grown the same three lines, and a
 * change to the host or to how the base is joined would have had to find both.
 */
export function at(path: string): URL {
  return new URL(`https://panel.example${basePath}${path}`);
}

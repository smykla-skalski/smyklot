import { describe, expect, it } from 'vitest';
import { fileAdjustmentHref, fileAdjustmentPath } from '../src/lib/file-adjustment-link';

describe('file adjustment navigation', () => {
  it.each(['.config/checks.yaml', 'docs/notes #1.md', 'config.toml', 'settings.jsonc'])(
    'round trips a repository-relative path without introducing a new route: %s',
    (path) => {
      const href = fileAdjustmentHref('/workspace/org/repositories/repo', path);
      const url = new URL(href, 'http://localhost');
      expect(url.pathname).toBe('/workspace/org/repositories/repo');
      expect(fileAdjustmentPath(url.hash)).toBe(path);
    },
  );
  it.each([
    '#other',
    '#file-sync=%',
    '#file-sync=../a.toml',
    '#file-sync=/a.toml',
    '#file-sync=a%5Cb.yaml',
    '#file-sync=a.txt',
  ])('does not offer an unsupported or malformed target: %s', (hash) =>
    expect(fileAdjustmentPath(hash)).toBeNull(),
  );
});

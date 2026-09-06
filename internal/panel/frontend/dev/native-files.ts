import type { SyncFile, SyncFileMerge } from '../src/lib/types.js';

/** Runnable native-format examples, shared by the mock and browser workflow proof. */
export const NATIVE_FILE_VARIANTS: ReadonlyArray<{
  format: 'yaml' | 'toml' | 'jsonc' | 'markdown';
  file: SyncFile;
  merge: SyncFileMerge;
}> = [
  {
    format: 'yaml',
    file: { path: '.config/quality.yaml', content: '# Shared checks\nenabled: true\nretries: 3\n' },
    merge: { path: '.config/quality.yaml', overrides: { retries: 4 } },
  },
  {
    format: 'toml',
    file: {
      path: '.config/quality.toml',
      content: '# Shared checks\nenabled = true\nretries = 3\n',
    },
    merge: { path: '.config/quality.toml', overrides: { retries: 4 } },
  },
  {
    format: 'jsonc',
    file: {
      path: '.config/quality.jsonc',
      content: '{\n  // Shared checks\n  "enabled": true,\n  "retries": 3\n}\n',
    },
    merge: { path: '.config/quality.jsonc', overrides: { retries: 4 } },
  },
  {
    format: 'markdown',
    file: {
      path: 'docs/LOCAL.md',
      content: '# Local development\n\n## Checks\n\nRun `mise run ci`\n',
    },
    // Omitted strategy deliberately exercises the backend's extension default.
    merge: {
      path: 'docs/LOCAL.md',
      sections: [
        { action: 'after', heading: '## Checks', content: 'Run the integration checks too' },
      ],
    },
  },
];

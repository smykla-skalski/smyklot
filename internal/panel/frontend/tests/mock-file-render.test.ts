import { describe, expect, it } from 'vitest';

import { renderMockSyncFile } from '../dev/mock-file-render';
import { defaultFormattingPolicy } from '../src/lib/formatting';

describe('development file renderer [Unit]', () => {
  it('applies placeholders, JSON merge edits, and ordered common formatting', () => {
    const policy = defaultFormattingPolicy();
    const rendered = renderMockSyncFile({
      path: 'renovate.json',
      draft_content: '{\n  "branch": "{{DEFAULT_BRANCH}}",\n  "labels": ["base"]\n}',
      default_branch: 'trunk',
      base_policy: policy,
      merge: {
        strategy: 'deep-merge',
        overrides: { labels: ['repository'] },
      },
      overlays: [
        { common: { line_ending: 'crlf', final_newline: 'insert' } },
        { common: { line_ending: 'lf' } },
      ],
    });

    expect(rendered).toEqual({
      valid: true,
      content: '{\n  "branch": "trunk",\n  "labels": [\n    "repository"\n  ]\n}\n',
      changed: true,
      diagnostics: [],
    });
  });

  it('compacts safe JSON arrays without rewriting surrounding presentation', () => {
    const policy = defaultFormattingPolicy();
    const rendered = renderMockSyncFile({
      path: 'renovate.json',
      draft_content: '{\n  "labels": [\n    "one",\n    "two"\n  ],\n  "enabled": true\n}',
      base_policy: policy,
      overlays: [{ json: { arrays: 'compact' } }],
    });

    expect(rendered).toEqual({
      valid: true,
      content: '{\n  "labels": ["one", "two"],\n  "enabled": true\n}',
      changed: true,
      diagnostics: [],
    });
  });

  it('fails closed when compact JSON arrays contain comments', () => {
    const rendered = renderMockSyncFile({
      path: 'settings.jsonc',
      draft_content: '{"labels": ["one", /* keep */ "two"]}',
      base_policy: defaultFormattingPolicy(),
      overlays: [{ json: { arrays: 'compact' } }],
    });

    expect(rendered).toMatchObject({
      valid: false,
      diagnostics: [
        {
          code: 'unsafe_formatting',
          message: 'compact JSON arrays cannot contain comments or multiline values',
        },
      ],
    });
  });

  it('keeps unknown formats byte-identical and diagnoses invalid policy data', () => {
    const policy = defaultFormattingPolicy();
    expect(
      renderMockSyncFile({ path: 'Makefile', draft_content: 'all:\n\ttrue', base_policy: policy }),
    ).toMatchObject({ valid: true, content: 'all:\n\ttrue', changed: false });
    expect(
      renderMockSyncFile({
        path: 'README.md',
        draft_content: '# Read me',
        base_policy: { ...policy, common: { ...policy.common, line_width: 1 } },
      }),
    ).toMatchObject({
      valid: false,
      diagnostics: [{ code: 'invalid_request', message: 'the render request is invalid' }],
    });
  });

  it('renders the Markdown section actions used by the development fixture', () => {
    const rendered = renderMockSyncFile({
      path: 'CONTRIBUTING.md',
      draft_content:
        '# Contributing\n\n## Commits\n\nUse `git commit`.\n\n### Making Changes\n\nRun `make check`.\n',
      base_policy: defaultFormattingPolicy(),
      merge: {
        strategy: 'markdown',
        sections: [
          { action: 'after', heading: '## Commits', content: '- Squash on merge' },
          {
            action: 'patch',
            heading: '### Making Changes',
            patches: [{ find: 'make check', replace: 'mise run check' }],
          },
        ],
      },
    });

    expect(rendered.valid).toBe(true);
    expect(rendered.content).toContain('- Squash on merge');
    expect(rendered.content).toContain('mise run check');
  });
});

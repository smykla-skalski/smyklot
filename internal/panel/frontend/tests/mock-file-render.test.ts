import { afterAll, describe, expect, it } from 'vitest';

import { GoFileRenderer } from '../dev/mock-file-render';
import { defaultFormattingPolicy } from '../src/lib/formatting';

const renderer = new GoFileRenderer();
const base = defaultFormattingPolicy();

afterAll(() => renderer.close());

describe('development Go file renderer [Integration]', () => {
  it('applies placeholders, semantic merge edits, and ordered formatting', async () => {
    const rendered = await renderer.render({
      path: 'renovate.json',
      draft_content: '{\n  "branch": "{{DEFAULT_BRANCH}}",\n  "labels": ["base"]\n}',
      default_branch: 'trunk',
      merge: { strategy: 'deep-merge', overrides: { labels: ['repository'] } },
      base_formatting: base,
      layers: [
        {
          source: 'template',
          formatting: { common: { line_ending: 'crlf', final_newline: 'insert' } },
        },
        { source: 'repository_path', formatting: { common: { line_ending: 'lf' } } },
      ],
      inherited_layers: 1,
    });

    expect(rendered).toMatchObject({
      valid: true,
      final_content: '{\n  "branch": "trunk",\n  "labels": ["repository"]\n}\n',
      matches_formatting: true,
      diagnostics: [],
    });
    expect(rendered.inherited_policy.common.line_ending).toBe('crlf');
    expect(rendered.effective_policy.common.line_ending).toBe('lf');
  });

  it('compacts safe JSON arrays without rewriting surrounding presentation', async () => {
    const rendered = await renderer.render({
      path: 'renovate.json',
      draft_content: '{\n  "labels": [\n    "one",\n    "two"\n  ],\n  "enabled": true\n}',
      merge: {},
      base_formatting: base,
      layers: [{ source: 'template', formatting: { json: { arrays: 'compact' } } }],
      inherited_layers: 0,
    });

    expect(rendered.final_content).toBe('{\n  "labels": ["one", "two"],\n  "enabled": true\n}\n');
  });

  it('fails closed when compact JSONC arrays contain comments', async () => {
    const rendered = await renderer.render({
      path: 'settings.jsonc',
      draft_content: '{"labels": ["one", /* keep */ "two"]}',
      merge: {},
      base_formatting: base,
      layers: [{ source: 'template', formatting: { json: { arrays: 'compact' } } }],
      inherited_layers: 0,
    });

    expect(rendered).toMatchObject({
      valid: false,
      diagnostics: [{ stage: 'format', code: 'invalid_document' }],
    });
  });

  it('terminates unknown formats and diagnoses invalid policies', async () => {
    await expect(
      renderer.render({
        path: 'Makefile',
        draft_content: 'all:\n\ttrue',
        merge: {},
        base_formatting: base,
        layers: [],
        inherited_layers: 0,
      }),
    ).resolves.toMatchObject({
      valid: true,
      final_content: 'all:\n\ttrue\n',
      matches_formatting: true,
    });
    await expect(
      renderer.render({
        path: 'README.md',
        draft_content: '# Read me',
        merge: {},
        base_formatting: { ...base, common: { ...base.common, line_width: 1 } },
        layers: [],
        inherited_layers: 0,
      }),
    ).resolves.toMatchObject({
      valid: false,
      diagnostics: [{ stage: 'policy', code: 'invalid_policy' }],
    });
  });

  it('renders the Markdown section actions used by the fixture', async () => {
    const rendered = await renderer.render({
      path: 'CONTRIBUTING.md',
      draft_content:
        '# Contributing\n\n## Commits\n\nUse `git commit`.\n\n### Making Changes\n\nRun `make check`.\n',
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
      base_formatting: base,
      layers: [],
      inherited_layers: 0,
    });

    expect(rendered.valid).toBe(true);
    expect(rendered.final_content).toContain('- Squash on merge');
    expect(rendered.final_content).toContain('mise run check');
  });
});

import { describe, expect, it } from 'vitest';

import { rewriteMockHtml } from '../dev/mock-html';

describe('mock HTML rewriting', () => {
  it('resolves every runtime sentinel in SvelteKit-rendered HTML', () => {
    const source = [
      '/__smyklot_panel_base__/asset.js',
      '__smyklot_panel_version__',
      '__smyklot_panel_service__',
      '__smyklot_panel_error__',
      '__smyklot_panel_noscript__',
    ].join('\n');

    const rewritten = rewriteMockHtml(source);

    expect(rewritten).not.toContain('__smyklot_panel_');
    expect(rewritten).toContain('/asset.js');
    expect(rewritten).toContain('dev');
    expect(rewritten).toContain('local mock service');
    expect(rewritten).toContain('The Smyklot panel needs JavaScript to run.');
  });
});

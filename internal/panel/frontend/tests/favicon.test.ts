import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

const index = readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const panelAvatar = readFileSync(new URL('../public/smyklot-avatar.png', import.meta.url));
const latestAvatar = readFileSync(
  new URL('../../../../assets/smyklot-avatar-256-transparent-wide.png', import.meta.url),
);

describe('panel favicon', () => {
  it('uses the latest wide transparent avatar behind a release-versioned URL', () => {
    expect(panelAvatar).toEqual(latestAvatar);
    expect(index).toContain(
      'href="/__smyklot_panel_base__/smyklot-avatar.png?v=__smyklot_panel_version__"',
    );
  });
});

describe('panel document metadata', () => {
  it('identifies the private app and configures mobile browser behavior', () => {
    expect(index).toContain('<meta name="application-name" content="SMYKLOT" />');
    expect(index).toContain('interactive-widget=resizes-content');
    expect(index).toContain('<meta name="format-detection" content="telephone=no" />');
    expect(index).toContain('content="noindex, nofollow, noarchive, nosnippet, noimageindex"');
  });

  it('preloads the primary font and declares exact icon dimensions', () => {
    expect(index).toContain('href="/src/assets/fonts/PlusJakartaSansLatinVF.woff2"');
    expect(index).toContain('as="font"');
    expect(index).toContain('crossorigin');
    expect(index).toContain('rel="apple-touch-icon"');
    expect(index.match(/sizes="256x256"/g)).toHaveLength(2);
  });
});

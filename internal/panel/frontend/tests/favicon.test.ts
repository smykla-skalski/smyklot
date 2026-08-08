import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

const index = readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const panelAvatar = readFileSync(new URL('../public/smyklot-avatar.png', import.meta.url));
const latestAvatar = readFileSync(
  new URL('../../../../assets/smyklot-avatar-256-dark.png', import.meta.url),
);

describe('panel favicon', () => {
  it('uses the latest avatar behind a release-versioned URL', () => {
    expect(panelAvatar).toEqual(latestAvatar);
    expect(index).toContain(
      'href="/__smyklot_panel_base__/smyklot-avatar.png?v=__smyklot_panel_version__"',
    );
  });
});

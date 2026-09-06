import { describe, expect, it } from 'vitest';

import { componentSources, markupOf } from './support/markup';

describe('shared form error layout', () => {
  it('does not let a route change every form error in the application', () => {
    const offenders = componentSources()
      .filter(([, source]) => {
        const styles = markupOf(source).match(/<style[^>]*>([\s\S]*?)<\/style>/u)?.[1] ?? '';

        return /(?:^|[{},])\s*:global\(\.form-error(?:[\s):.#[])/u.test(styles);
      })
      .map(([file]) => file);

    expect(offenders, 'own placement in a local wrapper, not an unscoped shared class').toEqual([]);
  });
});

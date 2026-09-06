import { describe, expect, it } from 'vitest';

import { componentSources, markupOf } from './support/markup';

describe('shared form control ownership', () => {
  it.each([
    'form-error',
    'select-wrap',
    'select-input',
    'text-input',
    'btn',
    'switch',
    'segmented',
  ])('does not let a route globally override shared %s controls', (sharedClass) => {
    const offenders = componentSources()
      .filter(([, source]) => {
        const styles = markupOf(source).match(/<style[^>]*>([\s\S]*?)<\/style>/u)?.[1] ?? '';

        return new RegExp(`(?:^|[{},])\\s*:global\\(\\.${sharedClass}(?:[\\s):.#\\[])`, 'u').test(
          styles,
        );
      })
      .map(([file]) => file);

    expect(offenders, 'own placement in a local wrapper, not an unscoped shared class').toEqual([]);
  });
});

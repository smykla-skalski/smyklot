import { describe, expect, it } from 'vitest';

import { contrast, deltaE } from './color';
import { palettes } from './theme';

describe('unsaved settings colors [Unit]', () => {
  for (const palette of palettes) {
    it(`keeps changed rows distinct and readable in ${palette.name}`, () => {
      const surface = palette.color('unsaved-surface');
      expect(deltaE(surface, palette.color('surface-base'))).toBeGreaterThan(5);
      for (const text of ['unsaved-ink', 'unsaved-secondary']) {
        expect(contrast(palette.color(text), surface), text).toBeGreaterThanOrEqual(4.5);
      }
      expect(palette.color('decision-accent')).toBe(palette.color('warning'));
      expect(
        contrast(palette.color('on-decision-accent'), palette.color('decision-accent')),
      ).toBeGreaterThanOrEqual(4.5);
    });
  }
});

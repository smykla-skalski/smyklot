/**
 * Panel bootstrapping shared by the root layout.
 *
 * Migrated from `main.ts`: legacy preference migration runs first, then the
 * stored theme is applied before the routed tree mounts, so the first paint
 * carries no theme flash.
 */
import {
  applyDocumentTheme,
  DEFAULT_THEME_DISPLAY,
  isThemeDisplay,
  resolveThemeDisplay,
} from './preferences';
import { effectivePref, migrateLegacyPreferences, readPrefsDoc } from './preferences-sync';

export function initializePanel(document: Document): void {
  migrateLegacyPreferences();
  const storedTheme = effectivePref(readPrefsDoc(), 'theme');
  const theme = resolveThemeDisplay(
    typeof storedTheme === 'string' && isThemeDisplay(storedTheme)
      ? storedTheme
      : DEFAULT_THEME_DISPLAY,
  );
  applyDocumentTheme(document, theme);
}

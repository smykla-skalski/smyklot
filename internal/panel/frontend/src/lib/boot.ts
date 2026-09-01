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

/**
 * The class the document wears until the page has finished arriving.
 *
 * Motion is how this product tells a reader that something CHANGED, and until the page
 * has settled nothing has: the first paint is a first placement, and the geometry that
 * follows it is the layout finding its answer, not an answer moving. Drawn as motion it
 * is a control that jitters on every reload - a segmented control's thumb slid 90px and
 * grew 19px on the queue page while the reader had touched nothing.
 */
const BOOTING_CLASS = 'is-booting';

export function isBooting(document: Document): boolean {
  return document.documentElement.classList.contains(BOOTING_CLASS);
}

/**
 * Let the page move again.
 *
 * Waits on the two things that actually change geometry after the first paint: the
 * routed tree's own mount, which the caller has already reached, and the webfonts, which
 * re-measure every control that is sized by its words. A frame after both is the first
 * moment at which a change is a change rather than the layout arriving.
 */
export async function panelHasSettled(document: Document): Promise<void> {
  await document.fonts?.ready;
  await new Promise((resolve) => requestAnimationFrame(resolve));
  await new Promise((resolve) => requestAnimationFrame(resolve));
  document.documentElement.classList.remove(BOOTING_CLASS);
}

export function initializePanel(document: Document): void {
  document.documentElement.classList.add(BOOTING_CLASS);
  migrateLegacyPreferences();
  const storedTheme = effectivePref(readPrefsDoc(), 'theme');
  const theme = resolveThemeDisplay(
    typeof storedTheme === 'string' && isThemeDisplay(storedTheme)
      ? storedTheme
      : DEFAULT_THEME_DISPLAY,
  );
  applyDocumentTheme(document, theme);
}

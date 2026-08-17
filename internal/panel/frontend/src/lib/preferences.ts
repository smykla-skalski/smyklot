export type TimeDisplay = 'relative' | 'absolute';
export type SidebarDisplay = 'expanded' | 'collapsed';
export type ThemeDisplay = 'system' | 'light' | 'dark';
export type ResolvedTheme = Exclude<ThemeDisplay, 'system'>;

export const DEFAULT_TIME_DISPLAY: TimeDisplay = 'relative';
export const DEFAULT_SIDEBAR_DISPLAY: SidebarDisplay = 'expanded';
export const DEFAULT_THEME_DISPLAY: ThemeDisplay = 'system';

const TIME_DISPLAY_KEY = 'smyklot.panel.history.time-display';
const LAST_INSTALLATION_KEY = 'smyklot.panel.last-installation';
const SIDEBAR_DISPLAY_KEY = 'smyklot.panel.sidebar.display';
const THEME_DISPLAY_KEY = 'smyklot.panel.theme';

type PreferenceReader = Pick<Storage, 'getItem'>;

/**
 * Which of the two the value should outlive.
 *
 * `local` is the browser's: a preference is the reader's answer everywhere they open the
 * panel. `session` is the tab's: where one tab came from is a fact about that journey,
 * and two tabs sitting in two places must not overwrite each other's.
 */
export type StorageLifetime = 'local' | 'session';

// Exported for the specs: the rule it states - unusable storage reads as null - is invisible from
// the readers, which catch the resulting failure either way.
export function browserStorage(lifetime: StorageLifetime = 'local'): Storage | null {
  if (typeof window === 'undefined') return null;

  try {
    // lib.dom types these as a plain `Storage`, but a host can leave the accessor answering
    // undefined instead of throwing - Node's own Web Storage does exactly that unless the process
    // was given `--localstorage-file`, and it shadows the storage jsdom would otherwise install.
    // Every caller guards on `null`, so anything unusable has to become null here; returning it
    // would slip past that guard and only be caught by the `catch` around the first `getItem`.
    const storage = (lifetime === 'local' ? window.localStorage : window.sessionStorage) as
      Storage | null | undefined;
    if (storage === null || storage === undefined) return null;

    return typeof storage.getItem === 'function' ? storage : null;
  } catch {
    return null;
  }
}

export function isTimeDisplay(value: string | null): value is TimeDisplay {
  return value === 'relative' || value === 'absolute';
}

export function isSidebarDisplay(value: string | null): value is SidebarDisplay {
  return value === 'expanded' || value === 'collapsed';
}

export function isThemeDisplay(value: string | null): value is ThemeDisplay {
  return value === 'system' || value === 'light' || value === 'dark';
}

export function systemThemeDisplay(): ResolvedTheme {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return 'light';
  }

  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

export function resolveThemeDisplay(
  value: ThemeDisplay,
  systemTheme: ResolvedTheme = systemThemeDisplay(),
): ResolvedTheme {
  return value === 'system' ? systemTheme : value;
}

export function themeColor(theme: ResolvedTheme, rootConsole = false): string {
  if (rootConsole) return theme === 'dark' ? '#0f0d14' : '#f3f1f7';
  return theme === 'dark' ? '#0e1116' : '#f5f7f6';
}

export function applyDocumentTheme(
  source: Document,
  theme: ResolvedTheme,
  rootConsole = false,
): void {
  source.documentElement.dataset.theme = theme;
  const color = themeColor(theme, rootConsole);
  for (const meta of source.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]')) {
    meta.content = color;
  }
}

export function readTimeDisplay(storage: PreferenceReader | null = browserStorage()): TimeDisplay {
  if (storage === null) return DEFAULT_TIME_DISPLAY;

  try {
    const value = storage.getItem(TIME_DISPLAY_KEY);
    return isTimeDisplay(value) ? value : DEFAULT_TIME_DISPLAY;
  } catch {
    return DEFAULT_TIME_DISPLAY;
  }
}

export function readLastInstallation(
  storage: PreferenceReader | null = browserStorage(),
): string | null {
  if (storage === null) return null;

  try {
    const value = storage.getItem(LAST_INSTALLATION_KEY)?.trim();
    return value === undefined || value === '' ? null : value;
  } catch {
    return null;
  }
}

export function readSidebarDisplay(
  storage: PreferenceReader | null = browserStorage(),
): SidebarDisplay {
  if (storage === null) return DEFAULT_SIDEBAR_DISPLAY;

  try {
    const value = storage.getItem(SIDEBAR_DISPLAY_KEY);
    return isSidebarDisplay(value) ? value : DEFAULT_SIDEBAR_DISPLAY;
  } catch {
    return DEFAULT_SIDEBAR_DISPLAY;
  }
}

export function readThemeDisplay(
  storage: PreferenceReader | null = browserStorage(),
  fallback: ThemeDisplay = DEFAULT_THEME_DISPLAY,
): ThemeDisplay {
  if (storage === null) return fallback;

  try {
    const value = storage.getItem(THEME_DISPLAY_KEY);
    return isThemeDisplay(value) ? value : fallback;
  } catch {
    return fallback;
  }
}

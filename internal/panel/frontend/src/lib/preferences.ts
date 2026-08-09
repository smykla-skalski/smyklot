export type TimeDisplay = 'relative' | 'absolute';
export type SidebarDisplay = 'expanded' | 'collapsed';
export type ThemeDisplay = 'light' | 'dark';

export const DEFAULT_TIME_DISPLAY: TimeDisplay = 'relative';
export const DEFAULT_SIDEBAR_DISPLAY: SidebarDisplay = 'expanded';
export const DEFAULT_THEME_DISPLAY: ThemeDisplay = 'light';

const TIME_DISPLAY_KEY = 'smyklot.panel.history.time-display';
const LAST_INSTALLATION_KEY = 'smyklot.panel.last-installation';
const SIDEBAR_DISPLAY_KEY = 'smyklot.panel.sidebar.display';
const THEME_DISPLAY_KEY = 'smyklot.panel.theme';

type PreferenceReader = Pick<Storage, 'getItem'>;
type PreferenceWriter = Pick<Storage, 'setItem'>;

function browserStorage(): Storage | null {
  if (typeof window === 'undefined') return null;

  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

function isTimeDisplay(value: string | null): value is TimeDisplay {
  return value === 'relative' || value === 'absolute';
}

function isSidebarDisplay(value: string | null): value is SidebarDisplay {
  return value === 'expanded' || value === 'collapsed';
}

function isThemeDisplay(value: string | null): value is ThemeDisplay {
  return value === 'light' || value === 'dark';
}

export function preferredThemeDisplay(): ThemeDisplay {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return DEFAULT_THEME_DISPLAY;
  }

  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
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

export function writeTimeDisplay(
  value: TimeDisplay,
  storage: PreferenceWriter | null = browserStorage(),
): void {
  if (storage === null) return;

  try {
    storage.setItem(TIME_DISPLAY_KEY, value);
  } catch {
    // Browser preferences are best-effort and must never block the panel
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

export function writeLastInstallation(
  account: string,
  storage: PreferenceWriter | null = browserStorage(),
): void {
  if (storage === null) return;

  try {
    storage.setItem(LAST_INSTALLATION_KEY, account);
  } catch {
    // Browser preferences are best-effort and must never block the panel
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

export function writeSidebarDisplay(
  value: SidebarDisplay,
  storage: PreferenceWriter | null = browserStorage(),
): void {
  if (storage === null) return;

  try {
    storage.setItem(SIDEBAR_DISPLAY_KEY, value);
  } catch {
    // Browser preferences are best-effort and must never block the panel
  }
}

export function readThemeDisplay(
  storage: PreferenceReader | null = browserStorage(),
  fallback: ThemeDisplay = preferredThemeDisplay(),
): ThemeDisplay {
  if (storage === null) return fallback;

  try {
    const value = storage.getItem(THEME_DISPLAY_KEY);
    return isThemeDisplay(value) ? value : fallback;
  } catch {
    return fallback;
  }
}

export function writeThemeDisplay(
  value: ThemeDisplay,
  storage: PreferenceWriter | null = browserStorage(),
): void {
  if (storage === null) return;

  try {
    storage.setItem(THEME_DISPLAY_KEY, value);
  } catch {
    // Browser preferences are best-effort and must never block the panel
  }
}

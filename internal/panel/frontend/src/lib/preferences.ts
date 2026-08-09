export type TimeDisplay = 'relative' | 'absolute';

export const DEFAULT_TIME_DISPLAY: TimeDisplay = 'relative';

const TIME_DISPLAY_KEY = 'smyklot.panel.history.time-display';
const LAST_INSTALLATION_KEY = 'smyklot.panel.last-installation';

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

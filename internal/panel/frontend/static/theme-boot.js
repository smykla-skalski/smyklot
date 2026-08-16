{
  let preference = 'system';
  try {
    const stored = localStorage.getItem('smyklot.panel.prefs');
    if (stored !== null) {
      const preferences = JSON.parse(stored);
      const pending = preferences?.pending;
      const shadow = preferences?.shadow;
      if (pending !== null && typeof pending === 'object' && 'theme' in pending) {
        preference = pending.theme ?? 'system';
      } else if (shadow !== null && typeof shadow === 'object' && 'theme' in shadow) {
        preference = shadow.theme;
      }
    } else {
      preference = localStorage.getItem('smyklot.panel.theme') ?? 'system';
    }
  } catch {
    // Preferences are best-effort; the system theme remains the fallback.
  }

  if (preference !== 'light' && preference !== 'dark') preference = 'system';
  const theme =
    preference === 'system'
      ? matchMedia('(prefers-color-scheme: dark)').matches
        ? 'dark'
        : 'light'
      : preference;
  const base =
    document.querySelector('meta[name="smyklot-panel-base"]')?.getAttribute('content') ?? '';
  const rootConsole = location.pathname.startsWith(`${base}/root`);
  const color = rootConsole
    ? theme === 'dark'
      ? '#0f0d14'
      : '#f3f1f7'
    : theme === 'dark'
      ? '#0e1116'
      : '#f5f7f6';
  document.documentElement.dataset.theme = theme;
  for (const meta of document.querySelectorAll('meta[name="theme-color"]')) {
    meta.setAttribute('content', color);
  }
}

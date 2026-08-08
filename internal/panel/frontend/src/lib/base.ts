/**
 * The build-time stand-in for the panel's mount point.
 *
 * Vite bakes `base` into the emitted asset URLs, but the mount point is a
 * runtime flag, so the build uses this sentinel and the serving binary
 * substitutes the configured prefix into `index.html`. Under `vite dev` no
 * substitution happens and the sentinel is the real mount point, which is why
 * reading it back is enough in both cases.
 */
export const BASE_PATH_SENTINEL = '/__harness_panel_base__';

const BASE_META_NAME = 'harness-panel-base';
const VERSION_META_NAME = 'harness-panel-version';
const DAEMON_META_NAME = 'harness-panel-daemon';

/** Build-time stand-ins, left as themselves under `vite dev`. */
const VERSION_SENTINEL = '__harness_panel_version__';
const DAEMON_SENTINEL = '__harness_panel_daemon__';

/**
 * Harness Monitor's icon, copied verbatim into the bundle root by Vite.
 *
 * Kept in step with the icon links in `index.html`, which is the other place it
 * is named.
 */
export const MONITOR_ICON_PATH = '/harness-monitor.png';

/**
 * Read the mount point the serving binary injected into `index.html`.
 *
 * An empty value is the origin root rather than a missing one: a panel mounted
 * there has the sentinel replaced with nothing at all. The tag being absent is
 * the only failure, which is why this asks whether the attribute is there and
 * not whether it says anything.
 */
export function readBasePath(source: Document): string {
  const meta = source.querySelector(`meta[name="${BASE_META_NAME}"]`);
  const content = meta?.getAttribute('content');
  if (content === null || content === undefined) {
    throw new Error(`the panel page is missing its <meta name="${BASE_META_NAME}"> element`);
  }
  return normalizeBasePath(content);
}

/**
 * What the page says about the panel that served it.
 *
 * The daemon's own version is not here: the panel's credential may mint and
 * manage pairings and nothing else, so the only answer that carries it is the
 * pairing list, which needs a session. It reaches the footer from there.
 */
export interface PanelBuild {
  /** The panel's own version, or `null` when the page was not served by one. */
  version: string | null;
  /** Host of the daemon this panel mints credentials from. */
  daemonHost: string | null;
}

/**
 * Read the build facts the serving binary injected.
 *
 * Both are optional by design. Under `vite dev` the sentinels are still in
 * place, and a page carrying one has nothing true to report, so it reports
 * nothing rather than printing a stand-in into the footer.
 */
export function readPanelBuild(source: Document): PanelBuild {
  return {
    version: readInjected(source, VERSION_META_NAME, VERSION_SENTINEL),
    daemonHost: readInjected(source, DAEMON_META_NAME, DAEMON_SENTINEL),
  };
}

function readInjected(source: Document, name: string, sentinel: string): string | null {
  const content = source.querySelector(`meta[name="${name}"]`)?.getAttribute('content')?.trim();
  if (content === undefined || content === '' || content === sentinel) {
    return null;
  }
  return content;
}

/**
 * Reduce a mount point to the one spelling the URL builder expects: a leading
 * slash and no trailing one, so joining never produces `//` or a bare relative
 * path that would resolve against whatever route the browser is showing.
 */
export function normalizeBasePath(raw: string): string {
  const trimmed = raw.trim().replace(/\/+$/, '');
  if (trimmed === '') {
    return '';
  }
  return trimmed.startsWith('/') ? trimmed : `/${trimmed}`;
}

/** Build an absolute path under the panel's mount point. */
export function panelUrl(base: string, path: string): string {
  const suffix = path.startsWith('/') ? path : `/${path}`;
  return `${normalizeBasePath(base)}${suffix}`;
}

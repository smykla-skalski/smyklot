/** Build-time mount placeholder replaced only in the generated index page. */
export const BASE_PATH_SENTINEL = '/__smyklot_panel_base__';

const BASE_META_NAME = 'smyklot-panel-base';
const VERSION_META_NAME = 'smyklot-panel-version';
const SERVICE_META_NAME = 'smyklot-panel-service';
const VERSION_SENTINEL = '__smyklot_panel_version__';
const SERVICE_SENTINEL = '__smyklot_panel_service__';

export const PANEL_ICON_PATH = '/smyklot-avatar.png';

export interface PanelBuild {
  version: string | null;
  serviceHost: string | null;
}

export function readBasePath(source: Document): string {
  const meta = source.querySelector(`meta[name="${BASE_META_NAME}"]`);
  const content = meta?.getAttribute('content');
  if (content === null || content === undefined) {
    throw new Error(`the panel page is missing its <meta name="${BASE_META_NAME}"> element`);
  }
  return normalizeBasePath(content);
}

export function readPanelBuild(source: Document): PanelBuild {
  return {
    version: readInjected(source, VERSION_META_NAME, VERSION_SENTINEL),
    serviceHost: readInjected(source, SERVICE_META_NAME, SERVICE_SENTINEL),
  };
}

function readInjected(source: Document, name: string, sentinel: string): string | null {
  const content = source.querySelector(`meta[name="${name}"]`)?.getAttribute('content')?.trim();
  if (content === undefined || content === '' || content === sentinel) {
    return null;
  }
  return content;
}

export function normalizeBasePath(raw: string): string {
  const trimmed = raw.trim().replace(/\/+$/, '');
  if (trimmed === '') {
    return '';
  }
  return trimmed.startsWith('/') ? trimmed : `/${trimmed}`;
}

export function panelUrl(base: string, path: string): string {
  const suffix = path.startsWith('/') ? path : `/${path}`;
  return `${normalizeBasePath(base)}${suffix}`;
}

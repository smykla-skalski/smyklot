const BASE_SENTINEL = '/__smyklot_panel_base__';
const VERSION_SENTINEL = '__smyklot_panel_version__';
const SERVICE_SENTINEL = '__smyklot_panel_service__';
const ERROR_SENTINEL = '__smyklot_panel_error__';
const NOSCRIPT_SENTINEL = '__smyklot_panel_noscript__';

const DEFAULT_NOSCRIPT = 'The Smyklot panel needs JavaScript to run.';

export function rewriteMockHtml(html: string): string {
  return html
    .replaceAll(BASE_SENTINEL, '')
    .replaceAll(VERSION_SENTINEL, 'dev')
    .replaceAll(SERVICE_SENTINEL, 'local mock service')
    .replaceAll(ERROR_SENTINEL, '')
    .replaceAll(NOSCRIPT_SENTINEL, DEFAULT_NOSCRIPT);
}

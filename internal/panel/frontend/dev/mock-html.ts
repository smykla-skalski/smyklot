const BASE_SENTINEL = '/__smyklot_panel_base__';
const VERSION_SENTINEL = '__smyklot_panel_version__';

/**
 * The version the mock dev server reports, which `vite.config.ts` also hands SvelteKit.
 *
 * Both, and not this rewrite alone. SvelteKit puts the version in the inline bootstrap
 * script and hashes that script into the CSP, so replacing a sentinel inside it
 * afterwards leaves a hash that no longer describes what is served and the browser
 * refuses to run the page at all - an empty body, with the reason only in the console.
 * Configuring the value means there is nothing left in that script to rewrite.
 */
export const MOCK_VERSION = 'dev';

/**
 * The base the mock serves under, which `vite.config.ts` also hands SvelteKit.
 *
 * Empty, because the mock mounts at the root. Both halves have to agree: SvelteKit
 * builds every address from the configured base, and this rewrite fills the sentinels
 * `app.html` spells by hand, so a base configured here and not there would leave the
 * page asking for `/__smyklot_panel_base__/theme-boot.js`.
 */
export const MOCK_BASE = '';

/**
 * Whether the mock API is serving.
 *
 * Read from the process rather than through `$app/env/private`, which the Vite config
 * and the mock server cannot reach. `src/env.ts` declares the same variable for the one
 * reader that can.
 */
export function mockEnabled(): boolean {
  return process.env.SMYKLOT_PANEL_DEV_MOCK === '1';
}
const SERVICE_SENTINEL = '__smyklot_panel_service__';
const ERROR_SENTINEL = '__smyklot_panel_error__';
const NOSCRIPT_SENTINEL = '__smyklot_panel_noscript__';

const DEFAULT_NOSCRIPT = 'The Smyklot panel needs JavaScript to run.';

export function rewriteMockHtml(html: string): string {
  return html
    .replaceAll(BASE_SENTINEL, MOCK_BASE)
    .replaceAll(VERSION_SENTINEL, MOCK_VERSION)
    .replaceAll(SERVICE_SENTINEL, 'local mock service')
    .replaceAll(ERROR_SENTINEL, '')
    .replaceAll(NOSCRIPT_SENTINEL, DEFAULT_NOSCRIPT);
}

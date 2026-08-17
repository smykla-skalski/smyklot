import type { Plugin } from 'vite';

/**
 * Fails the build when a placeholder is emitted in a shape the server cannot resolve.
 *
 * The Go server fills these in at startup, and for every asset but `index.html` it does
 * so only where one stands as a **complete string literal** - quote, placeholder, quote.
 * That narrowness is deliberate: substituting a placeholder buried in a template would
 * mean guessing how to escape the value for the context around it, so anything else is
 * left alone and a fail-closed check refuses the bundle instead.
 *
 * Which means a build can emit an unservable bundle, and did. The service worker took
 * its version from a Vite `define`; interpolating it into a template let the minifier
 * fold the value inside, the placeholder lost its opening quote, and the service refused
 * to start. Nothing said so until the Go tests ran.
 *
 * So the build says so, where the shape is decided. Written as its own check rather than
 * inferred from the server's behaviour, because the two run in different languages and
 * the server's copy is the one that has to stay narrow.
 */
const SENTINELS = [
  '/__smyklot_panel_base__',
  '__smyklot_panel_version__',
  '__smyklot_panel_service__',
];

const DELIMITERS = new Set(['"', "'", '`']);

/** Assets the server rewrites wholesale rather than literal by literal. */
const REWRITTEN_WHOLE = new Set(['index.html']);

/**
 * The builds whose output is served.
 *
 * The SSR build runs to prerender the shell and is then discarded, so what it emits is
 * never rewritten and never checked.
 */
const SERVED = new Set(['client', 'serviceWorker']);

export function checkSentinels(): Plugin {
  return {
    name: 'smyklot-panel-sentinels',
    apply: 'build',
    generateBundle(_options, bundle) {
      if (!SERVED.has(this.environment.name)) return;

      for (const [fileName, chunk] of Object.entries(bundle)) {
        if (REWRITTEN_WHOLE.has(fileName)) continue;
        const source = chunk.type === 'chunk' ? chunk.code : chunk.source;
        if (typeof source !== 'string') continue;

        for (const sentinel of SENTINELS) {
          for (let at = source.indexOf(sentinel); at !== -1;) {
            const opening = source[at - 1];
            const closing = source[at + sentinel.length];
            if (opening === undefined || !DELIMITERS.has(opening) || opening !== closing) {
              this.error(
                `${fileName} emits ${sentinel} outside a complete string literal, so the ` +
                  'server cannot resolve it and will refuse the bundle. Build the value ' +
                  'by concatenation rather than interpolation - a template lets the ' +
                  'minifier fold it in and take the quotes off.',
              );
            }
            at = source.indexOf(sentinel, at + sentinel.length);
          }
        }
      }
    },
  };
}

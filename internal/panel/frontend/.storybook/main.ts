import { fileURLToPath } from 'node:url';

import type { StorybookConfig } from '@storybook/sveltekit';

const stories = fileURLToPath(new URL('../stories', import.meta.url));

/**
 * The catalogue reads the panel's own `vite.config.ts`.
 *
 * Storybook's builder loads it through Vite's `loadConfigFromFile`, so the whole
 * `sveltekit({...})` call runs and `#lib/*`, `$app/paths` and the preprocessing
 * work here exactly as they do in the app. `@storybook/sveltekit` then drops the
 * two plugins that would try to build a SvelteKit app - `vite-plugin-sveltekit-compile`
 * and `vite-plugin-sveltekit-guard` - and aliases `$app/*` to its own mocks.
 *
 * There is no `svelte.config.js` in this repository and none is needed: every Kit
 * option is inlined into that plugin call, and Storybook's Svelte preset does not
 * read one.
 *
 * Stories live in `stories/`, beside `tests/` and `dev/`, rather than next to the
 * components. Fifteen unit tests sweep `src/` by directory listing and assert rules
 * written for app source - `tests/csp-safety.test.ts` rejects a `style=` attribute,
 * `tests/surfaces.test.ts` builds its orphan-token set from what it finds - so a
 * story under `src/` would be judged by rules it was never meant to answer, and
 * would quietly widen the token sweep with tokens the app does not paint.
 */
const config: StorybookConfig = {
  framework: '@storybook/sveltekit',
  stories: ['../stories/**/*.mdx', '../stories/**/*.stories.svelte'],
  // No theme addon: the two palette axes are `globalTypes` in `preview.ts`, applied
  // through the panel's own `applyDocumentTheme`. An addon would set the data
  // attribute and stop there, leaving the `theme-color` metas saying the opposite
  // of what the page is painting.
  addons: ['@storybook/addon-svelte-csf', '@storybook/addon-docs', '@storybook/addon-a11y'],
  // `theme-boot.js`, the halo and the avatar are referenced by BrandMark, PageFooter
  // and `app.html`, so the catalogue has to serve the same directory the app does.
  staticDirs: ['../static'],

  viteFinal(config) {
    // SvelteKit pins `server.fs.allow` to the directories an app is built from -
    // `src`, `src/lib`, `.svelte-kit`, `node_modules` - and Storybook adds `.storybook`
    // to it. `stories/` is on neither list, so every story module comes back 403 and
    // the preview reports it as a failed dynamic import rather than as a refusal.
    const server = (config.server ??= {});
    const fs = (server.fs ??= {});
    fs.allow = [...(fs.allow ?? []), stories];
    return config;
  },
};

export default config;

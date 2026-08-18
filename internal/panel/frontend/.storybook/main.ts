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
  // `@storybook/addon-mcp` runs an MCP server inside the dev server, at
  // `http://localhost:6008/mcp` - the port this catalogue serves on, not Storybook's
  // default 6006, which is what `.mcp.json` at the repository root points at.
  //
  // It reaches `valibot` through `@storybook/mcp`, and the tree it asks for is
  // vulnerable to GHSA-5qjj-4xww-7phc - `package.json` pins the fixed 1.4.2 with an
  // `overrides` entry, which is the same narrow instrument the `runed` peer uses.
  //
  // Its docs toolset is React-only today, so what a Svelte project gets from it is
  // the development half: which stories a local change touches, how this project
  // writes one, and a rendered preview. `run-story-tests` wants
  // `@storybook/addon-vitest`, which is an optional peer and not installed - the
  // browser suites are where this repository measures.
  addons: [
    '@storybook/addon-svelte-csf',
    '@storybook/addon-docs',
    '@storybook/addon-a11y',
    '@storybook/addon-mcp',
  ],
  // `theme-boot.js`, the halo and the avatar are referenced by BrandMark, PageFooter
  // and `app.html`, so the catalogue has to serve the same directory the app does.
  staticDirs: ['../static'],

  viteFinal(config, { configType }) {
    // SvelteKit pins `server.fs.allow` to the directories an app is built from -
    // `src`, `src/lib`, `.svelte-kit`, `node_modules` - and Storybook adds `.storybook`
    // to it. `stories/` is on neither list, so every story module comes back 403 and
    // the preview reports it as a failed dynamic import rather than as a refusal.
    const server = (config.server ??= {});
    const fs = (server.fs ??= {});
    fs.allow = [...(fs.allow ?? []), stories];

    // `vite-plugin-sveltekit-setup` declares `__SVELTEKIT_PAYLOAD__` in dev and not in
    // a build, where Kit's own client config declares it instead - and that half never
    // runs here, because `@storybook/sveltekit` drops the compile plugin that invokes
    // it. `client/payload.js` reads the global at module scope as
    // `__SVELTEKIT_PAYLOAD__ ?? {}`, and `??` does not rescue an *undeclared*
    // identifier: it throws a ReferenceError that takes the whole preview bundle down
    // before one story renders, while `storybook build` still exits 0. `undefined` is
    // the literal Kit's own client build substitutes under the default split strategy.
    if (configType === 'PRODUCTION') {
      config.define = { ...config.define, __SVELTEKIT_PAYLOAD__: 'undefined' };
    }
    return config;
  },
};

export default config;

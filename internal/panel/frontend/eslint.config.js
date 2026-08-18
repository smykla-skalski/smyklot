import js from '@eslint/js';
import svelte from 'eslint-plugin-svelte';
import globals from 'globals';
import tseslint from 'typescript-eslint';

export default tseslint.config(
  { ignores: ['dist/**', 'node_modules/**', '.svelte-kit/**', 'storybook-static/**'] },
  js.configs.recommended,
  tseslint.configs.recommended,
  svelte.configs.recommended,
  {
    languageOptions: {
      globals: { ...globals.browser },
    },
  },
  {
    files: ['**/*.svelte', '**/*.svelte.ts'],
    languageOptions: {
      parserOptions: {
        parser: tseslint.parser,
      },
    },
  },
  {
    files: [
      '*.config.ts',
      '*.config.js',
      'tests/**/*.ts',
      'dev/**/*.ts',
      'build/**/*.ts',
      // `main.ts` is read by the Storybook CLI in Node; `preview.ts` is not, but it
      // sits beside it and nothing there wants the browser globals withheld.
      '.storybook/**/*.ts',
    ],
    languageOptions: {
      globals: { ...globals.node },
    },
  },
  {
    // A story for a component with a required snippet prop has to take it out of
    // the args spread, or `<Chip {...args}>Enabled</Chip>` sets `children` twice and
    // the markup between the tags loses. `{ children, ...args }` is how that is
    // written, and it leaves a binding nothing reads - which is exactly the case
    // `ignoreRestSiblings` exists for.
    files: ['stories/**/*.svelte'],
    rules: {
      '@typescript-eslint/no-unused-vars': ['error', { ignoreRestSiblings: true }],
    },
  },
  {
    // Address generation goes through the panel's own route helpers
    // (`panelRoutePath`), which know the installation/root console grammar.
    // `resolve()` would only accept literal templates, so the href checks
    // SvelteKit turns on would flag every navigation in the app.
    rules: {
      'svelte/no-navigation-without-resolve': 'off',
    },
  },
  {
    // `package.json` retargets runed's declared `@sveltejs/kit` peer, which still
    // names ^2.21.0, at whatever Kit the panel pins. That override is safe for one
    // checkable reason: runed's only Kit-coupled module is `useSearchParams`, which
    // still calls `goto` with SvelteKit 2's options (svecosystem/runed#428), and it
    // reaches nothing here because runed's barrel does not re-export it. This holds
    // that reason as a rule rather than as a paragraph somebody has to remember.
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              group: ['$lib', '$lib/*'],
              message:
                'SvelteKit 3 removed `$lib`. Use the `#lib` subpath declared in package.json, ' +
                'with a file extension - `#lib/session.svelte.js`.',
            },
          ],
          paths: [
            {
              name: 'runed/kit',
              message:
                "runed/kit is built against SvelteKit 2's `goto` options. The `runed` " +
                'peer override in package.json assumes nothing imports it - see CLAUDE.md.',
            },
          ],
        },
      ],
    },
  },
);

import js from '@eslint/js';
import svelte from 'eslint-plugin-svelte';
import globals from 'globals';
import tseslint from 'typescript-eslint';

export default tseslint.config(
  { ignores: ['dist/**', 'node_modules/**', '.svelte-kit/**'] },
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
    files: ['*.config.ts', '*.config.js', 'tests/**/*.ts', 'dev/**/*.ts', 'build/**/*.ts'],
    languageOptions: {
      globals: { ...globals.node },
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
);

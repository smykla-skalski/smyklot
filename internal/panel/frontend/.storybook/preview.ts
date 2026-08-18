import type { Preview } from '@storybook/sveltekit';

import PanelShell from './PanelShell.svelte';

// The panel's one global stylesheet. The app pulls it in from `+layout.svelte`;
// nothing renders recognisably without it, and it carries the four `@font-face`
// blocks as well as every token.
import '../src/app.css';

/**
 * Two axes, four palettes.
 *
 * `app.css` declares the light palette on `:root`, the dark one on
 * `:root[data-theme='dark']`, and then declares the whole alias set twice more for
 * the Root console - once on `.app-shell.root-mode` and once again for dark. They
 * are four separate palettes rather than two, so both toolbars have to exist or
 * half the panel's colours are never looked at.
 */
const preview: Preview = {
  globalTypes: {
    theme: {
      description: 'Panel theme',
      toolbar: {
        title: 'Theme',
        icon: 'paintbrush',
        items: [
          { value: 'light', title: 'Light' },
          { value: 'dark', title: 'Dark' },
          { value: 'system', title: 'System' },
        ],
        dynamicTitle: true,
      },
    },
    console: {
      description: 'Which console the component is standing in',
      toolbar: {
        title: 'Console',
        icon: 'admin',
        items: [
          { value: 'panel', title: 'Panel' },
          { value: 'root', title: 'Root console' },
        ],
        dynamicTitle: true,
      },
    },
  },
  initialGlobals: { theme: 'dark', console: 'panel' },

  decorators: [
    (_story, context) => ({
      Component: PanelShell,
      props: {
        theme: context.globals.theme,
        console: context.globals.console,
      },
    }),
  ],

  parameters: {
    controls: { matchers: { color: /(background|color)$/iu, date: /Date$/u } },
    a11y: { test: 'todo' },
  },
};

export default preview;

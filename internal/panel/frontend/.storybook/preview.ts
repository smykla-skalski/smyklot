import type { Preview } from '@storybook/sveltekit';

import PanelShell from './PanelShell.svelte';
import { NOW } from '../stories/support/fixtures.js';

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
  /* Every catalogue fixture is derived from NOW. Freeze only the story iframe's
     wall clock so relative labels and countdowns describe that same instant, then
     restore it before Storybook switches stories. */
  beforeEach: () => {
    const liveNow = Date.now;
    Date.now = () => NOW;
    return () => {
      Date.now = liveNow;
    };
  },

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
        // `parameters: { bleed: true }` for the page backdrops, which are sized
        // `100vw` and belong to the window rather than to the content column.
        bleed: context.parameters.bleed === true,
      },
    }),
  ],

  parameters: {
    controls: { matchers: { color: /(background|color)$/iu, date: /Date$/u } },
    a11y: { test: 'todo' },
  },
};

export default preview;

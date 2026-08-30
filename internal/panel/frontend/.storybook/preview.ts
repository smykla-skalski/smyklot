import type { Preview } from '@storybook/sveltekit';
import contracts from 'virtual:component-contracts';

import PanelShell from './PanelShell.svelte';
import { NOW } from '../stories/support/fixtures.js';

// The panel's one global stylesheet. The app pulls it in from `+layout.svelte`;
// nothing renders recognisably without it, and it carries the four `@font-face`
// blocks and, through its `@import`, every token in `src/tokens.css`.
import '../src/app.css';

// The Docs page's own chrome, which Storybook hardcodes light. Loaded after the
// panel's stylesheet because it reads its tokens.
import './docs.css';

/**
 * Two axes, four palettes.
 *
 * `tokens.css` declares the light palette on `:root`, the dark one on
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

    /* The component's own `<!-- @component -->` block, shown at the head of its Docs
       page. Set here rather than as `docs.description.component` on each story: a
       contract restated in the catalogue is a second copy of it, and the copy is what
       goes stale. Storybook's Svelte docgen carries the props and no component
       description, so the name it does carry is what the contract is looked up by. */
    docs: {
      extractComponentDescription: (component: unknown): string | null => {
        const name = (component as { __docgen?: { name?: string } } | null)?.__docgen?.name;
        if (name === undefined) return null;
        return contracts[name.replace(/\.svelte$/u, '')] ?? null;
      },
    },
  },

  /* Every component gets a Docs page, so the contract in its `<!-- @component -->`
     block is read where the component is looked at rather than only where it is
     edited. Set here rather than per story: a catalogue where documentation is
     opt-in documents whatever somebody remembered to opt in. */
  tags: ['autodocs'],
};

export default preview;

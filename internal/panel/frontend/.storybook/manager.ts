import { addons } from 'storybook/manager-api';
import { create } from 'storybook/theming';

/**
 * Storybook's own chrome, in the panel's palette.
 *
 * The shipped themes are fine and belong to no product: a catalogue dressed in them
 * puts the panel's petrol and near-black inside a frame that shares nothing with it,
 * and every judgement about a colour is then made against a border that is not ours.
 * These are the panel's own dark primitives, read out of `app.css` rather than picked
 * to look similar - `--primitive-dark-canvas` for the ground the sidebar sits on,
 * `--primitive-dark-surface` for the panels over it, `--primitive-petrol-400` for the
 * one accent, because at this ground the 700 is too dark to read as a selection.
 *
 * Dark rather than following the toolbar. The theme control switches what a STORY
 * draws, and the chrome around it has to hold still: a frame that changed with every
 * story would make the story look like it moved. A stable dark surround is also the
 * quieter ground to judge a light story against - a light frame around a dark one
 * glares, and the reverse does not.
 *
 * Hex, not `var(--…)`: the manager renders outside the preview iframe and never loads
 * `app.css`, so a custom property here resolves to nothing. Keep them in step by hand
 * if the primitives move; `tests/design-tokens.test.ts` does not sweep this file,
 * because it is Storybook's UI and not the product's.
 */
export default addons.setConfig({
  theme: create({
    base: 'dark',

    brandTitle: 'Smyklot panel',
    brandUrl: 'https://smyklot.com',
    brandTarget: '_self',

    colorPrimary: '#2dd4bf',
    colorSecondary: '#2dd4bf',

    appBg: '#0e1116',
    appContentBg: '#151a21',
    appPreviewBg: '#151a21',
    appBorderColor: '#2a323d',
    appBorderRadius: 8,

    textColor: '#f3f6f7',
    textInverseColor: '#0e1116',
    textMutedColor: '#95a1ab',

    barTextColor: '#c4cdd4',
    barSelectedColor: '#2dd4bf',
    barHoverColor: '#2dd4bf',
    barBg: '#0e1116',

    inputBg: '#1a2029',
    inputBorder: '#3a4653',
    inputTextColor: '#f3f6f7',
    inputBorderRadius: 6,

    fontBase: "'Plus Jakarta Sans', 'Segoe UI Variable', 'Segoe UI', system-ui, sans-serif",
    fontCode: "'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, monospace",
  }),
});

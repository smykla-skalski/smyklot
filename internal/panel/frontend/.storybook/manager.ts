import { addons } from 'storybook/manager-api';
import { create } from 'storybook/theming';

/**
 * Storybook's own chrome, in the panel's palette, following the theme control.
 *
 * The shipped themes are fine and belong to no product: a catalogue dressed in them
 * puts the panel's petrol and near-black inside a frame that shares nothing with it,
 * and every judgement about a colour is then made against a border that is not ours.
 * These are the panel's own primitives, read out of `tokens.css` rather than picked to
 * look similar - `--primitive-*-canvas` for the ground the sidebar sits on,
 * `--primitive-*-surface` for the panels over it, and one petrol for the accent: the
 * 400 on the dark ground, where the 700 is too dark to read as a selection, and the
 * 700 on the light one, where the 400 is too pale.
 *
 * This used to be pinned dark, on the argument that a frame which changed with every
 * story would make the story look like it moved. That is a real effect and it is not
 * what the control does: the toolbar is set once and left, so the chrome moves when a
 * person changes mode and at no other time - and a dark frame around a light story
 * meant the whole catalogue could never be seen the way a light reader sees the app.
 *
 * Hex, not `var(--…)`: the manager renders outside the preview iframe and never loads
 * `app.css`, so a custom property here resolves to nothing. Keep them in step by hand
 * if the primitives move; `tests/design-tokens.test.ts` does not sweep this file,
 * because it is Storybook's UI and not the product's.
 */
const brand = {
  brandTitle: 'Smyklot panel',
  brandUrl: 'https://smyklot.com',
  brandTarget: '_self' as const,
  appBorderRadius: 8,
  inputBorderRadius: 6,
  fontBase: "'Plus Jakarta Sans', 'Segoe UI Variable', 'Segoe UI', system-ui, sans-serif",
  fontCode: "'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, monospace",
};

const dark = create({
  ...brand,
  base: 'dark',

  colorPrimary: '#2dd4bf',
  colorSecondary: '#2dd4bf',

  appBg: '#0e1116',
  appContentBg: '#151a21',
  appPreviewBg: '#151a21',
  appBorderColor: '#2a323d',

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
});

const light = create({
  ...brand,
  base: 'light',

  colorPrimary: '#0f766e',
  colorSecondary: '#0f766e',

  appBg: '#e6ede9',
  appContentBg: '#ffffff',
  appPreviewBg: '#ffffff',
  appBorderColor: '#e3e8e6',

  textColor: '#18211f',
  textInverseColor: '#f5f7f6',
  textMutedColor: '#63716c',

  barTextColor: '#4c5b57',
  barSelectedColor: '#0f766e',
  barHoverColor: '#0f766e',
  barBg: '#f5f7f6',

  inputBg: '#f8faf9',
  inputBorder: '#c4cec9',
  inputTextColor: '#18211f',
});

/**
 * `system` is a real third answer rather than a synonym for one of the two, so it is
 * resolved the way the panel resolves it - against the reader's own setting.
 */
function themeFor(display: unknown): ReturnType<typeof create> {
  if (display === 'light') return light;
  if (display === 'dark') return dark;
  return globalThis.matchMedia?.('(prefers-color-scheme: light)').matches === true ? light : dark;
}

addons.setConfig({ theme: dark });

/*
 * The toolbar's `theme` global lives in the preview; the manager only hears about it
 * over the channel. `SET_GLOBALS` carries the value the page loaded with - without it
 * a reload comes back to whichever theme was hardcoded above - and `GLOBALS_UPDATED`
 * carries every change after that.
 */
addons.register('smyklot/manager-theme', (api) => {
  const apply = (payload: { globals?: Record<string, unknown> }): void => {
    api.setOptions({ theme: themeFor(payload.globals?.theme) });
  };
  const channel = api.getChannel();
  channel?.on('setGlobals', apply);
  channel?.on('globalsUpdated', apply);
});

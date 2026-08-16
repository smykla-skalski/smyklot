import { readFileSync, readdirSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

/**
 * The theme switch was declared inside the sidebar, so the invitation page could not offer one
 * without a second copy of the options - the same shape that let the invitation page's wordmark
 * drift away from the sidebar's. `ThemeSwitch` is now the only definition.
 *
 * Checked as source, because the runtime here has no DOM and no cascade.
 */

const components = new URL('../src/lib/components/', import.meta.url);

const sources = readdirSync(components)
  .filter((file) => file.endsWith('.svelte'))
  .map((file) => [file, readFileSync(new URL(file, components), 'utf8')] as const);

const read = (file: string): string => sources.find(([name]) => name === file)?.[1] ?? '';
const appCss = readFileSync(new URL('../src/app.css', import.meta.url), 'utf8');

describe('the theme switch', () => {
  it('is the only component naming the theme options', () => {
    const holders = sources
      .filter(([, source]) => source.includes("label: 'System theme'"))
      .map(([file]) => file);

    expect(holders).toEqual(['ThemeSwitch.svelte']);
  });

  it('is what the sidebar and the pages outside the panel both render', () => {
    // `NightPage` is the shell the invitation and the error pages share, so it is the one
    // place outside the sidebar that renders a switch.
    for (const file of ['IdentityBar.svelte', 'NightPage.svelte']) {
      expect(read(file)).toMatch(/<ThemeSwitch\b/u);
    }
  });

  it('offers "follow the system" only where the answer is kept', () => {
    // A page outside the panel has no account behind it, so "system" there is an offer to follow
    // something it will forget. It asks for a theme outright instead; the sidebar still offers it.
    expect(read('NightPage.svelte')).toMatch(/<ThemeSwitch[^>]*\ssystem=\{false\}/su);
    expect(read('IdentityBar.svelte')).not.toMatch(/<ThemeSwitch[^>]*\ssystem=/su);
  });

  it('is on every page outside the panel, without exception', () => {
    // The error pages went without one for a while, on the reasoning that there is nothing on an
    // error page to settle into. It read as a page missing a piece: the head row is a title and a
    // control, and with the control gone the title floated. `NightPage` renders it unconditionally
    // now, so no page can be the odd one out.
    const page = read('NightPage.svelte');

    expect(page).toMatch(/<ThemeSwitch\b/u);
    expect(page, 'the switch must not be behind a condition').not.toMatch(
      /\{#if[^}]*\}\s*<ThemeSwitch/u,
    );
    for (const file of ['ErrorPage.svelte', 'InvitationPage.svelte', 'SignInPage.svelte']) {
      expect(read(file), `${file} must not opt out`).not.toContain('themeChoice');
    }
  });

  it('does not follow the system on the pages that cannot remember', () => {
    // A live `MediaQuery` out here would repaint the page under a reader midway through an
    // invitation because their laptop reached sunset, and these are the pages with no account
    // behind them to remember what they would rather have. The system theme is read once, as an
    // opening choice. The page's own comment says so in words, so this asks what it *imports*.
    const page = read('NightPage.svelte');

    expect(page).not.toMatch(/^\s*import\b.*\bMediaQuery\b/mu);
    expect(page).toContain('systemThemeDisplay()');
  });

  it('can be sized by the surface it stands on', () => {
    // `compact` is more specific than the base rule, so naming the token outright there would
    // silently drop a caller's height on every compact control - which is all of them.
    const control = read('SegmentedControl.svelte');
    const compact = /fieldset\.compact\s*\{([^}]*)\}/u.exec(control)?.[1] ?? '';

    expect(compact).toMatch(/height:\s*var\(--local-control-height,/u);
  });
});

describe('the night surface', () => {
  it('declares every token the control reads from it', () => {
    // A misspelt custom property resolves to nothing and the control paints untinted glass, which
    // is a plausible-looking result rather than a visible break.
    const referenced = new Set(
      [...read('SegmentedControl.svelte').matchAll(/var\((--night-seg-[a-z-]+)\)/gu)].map(
        (match) => match[1],
      ),
    );

    expect(referenced.size).toBeGreaterThan(0);
    for (const token of referenced) {
      expect(appCss, `${token} is read but never declared`).toContain(`${token}:`);
    }
  });

  it('is the same in both themes', () => {
    // The sky it stands on is night whichever theme the page is in. A per-theme declaration would
    // turn the control into a bright chip on a dark ground in light mode.
    const rules = [...appCss.matchAll(/([^{}]+)\{([^{}]*)\}/gu)].map(
      ([, selector, body]) => [selector ?? '', body ?? ''] as const,
    );
    const declaring = rules.filter(([, body]) => /--night-seg-[a-z-]+:/u.test(body));
    const themed = declaring
      .filter(([selector]) => selector.includes('data-theme'))
      .map(([selector]) => selector.trim());

    expect(declaring.length).toBe(1);
    expect(themed).toEqual([]);
  });
});

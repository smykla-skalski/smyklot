import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

import { stylesheets } from './theme';

const source = (path: string): string =>
  readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8');

function token(css: string, name: string): number {
  const value = new RegExp(`${name}:\\s*(\\d+)`, 'u').exec(css)?.[1];
  if (value === undefined) throw new Error(`${name} is not declared`);
  return Number(value);
}

describe('the responsive shell [Unit]', () => {
  /* NOTHING IS GLUED. A page scrolls as a page, on every route.
     ------------------------------------------------------------------------
     Four list views used to pin the pane to the viewport and scroll their rows
     inside it, so a reader crossing from one of those to any other page met a
     head that stayed put and then a head that did not.

     This used to be guarded by asking a session getter, which by then answered
     `false` unconditionally - a rule proved by reading its own constant. The
     getter is gone and so is the class it drove, so the rule is asked of the
     shell stylesheet: nothing there is FIXED to the viewport. The two things that
     do glue - the plan's apply bar and the settings index - are `position: sticky`
     inside a page that scrolls, and neither states a height.

     A bare `height`, not `min-height`: the shell and the workspace are both at
     least as tall as the viewport, which is what puts the footer at the bottom of
     a short page. That is a floor and it is what a page does. What glued was the
     pair - an exact `100dvh` with `overflow: hidden`, which caps the page at the
     window and moves the scrolling inside it. A dialog may still be viewport-tall;
     it is not the page. */
  it('fixes no part of the shell to the viewport height', () => {
    const shell = source('app.css');
    const glued = [...shell.matchAll(/^\s*(?:block-size|height):\s*100dvh/gmu)];

    expect(glued.map((one) => one[0].trim())).toEqual([]);
  });

  it('keeps the rail below dialogs and above the mobile drawer', () => {
    const css = stylesheets;
    const rail = source('lib/components/Rail.svelte');

    expect(token(css, '--layer-rail')).toBeGreaterThan(40);
    expect(token(css, '--layer-rail')).toBeLessThan(token(css, '--layer-dialog'));
    expect(rail).toMatch(/\.rail\s*\{[^}]*z-index:\s*var\(--layer-rail\)/su);
  });

  /* This used to assert that a stacked table row relaxed its stated height on a
     phone. There is no table here now: a person is a sentence in a list, and a row
     that grows around its own words needs no breakpoint to be told so. What is
     worth holding is that it stays that way.

     Named as `<table`, not as the component that used to draw one - that component
     is gone, so naming it would be a rule that passes because its subject does not
     exist. A markup element cannot be deleted out from under the assertion. */
  it('reads the console user list as rows rather than as a table', () => {
    const access = source('lib/components/RootAccess.svelte');

    expect(access).toContain('<ul class="object-list">');
    expect(access).not.toContain('<table');
  });

  it('gives the mobile NightPage switch a row of its own', () => {
    const night = source('lib/components/NightPage.svelte');

    expect(night).toMatch(
      /@media \(max-width: 36rem\)[^{]*\{\s*\.night-head\s*\{[^}]*flex-direction:\s*column/su,
    );
  });

  it('renders route failures through the shared recovery page', () => {
    const errorRoute = source('routes/+error.svelte');

    expect(errorRoute).toMatch(/import ErrorPage from/u);
    expect(errorRoute).toMatch(/<ErrorPage\b/u);
    expect(errorRoute).not.toContain('style="padding: 2rem');
  });
});

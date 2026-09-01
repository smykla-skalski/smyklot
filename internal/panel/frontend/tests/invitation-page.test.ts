import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

/**
 * The invitation page is read by someone who has not signed in, may never have heard of Smyklot,
 * and is being asked to authorise something. Three things it must not lose.
 *
 * Checked as source because the runtime here has no DOM. Narrow on purpose: each assertion names a
 * thing that would silently stop working, not a shape the markup happens to have today.
 */

const page = readFileSync(
  new URL('../src/lib/components/InvitationPage.svelte', import.meta.url),
  'utf8',
);
const rootLayout = readFileSync(new URL('../src/routes/+layout.svelte', import.meta.url), 'utf8');

/**
 * Every link on the page, with its attributes.
 *
 * All three spellings, because a link here is written whichever way it has to look:
 * `<Link>` for the two GitHub profiles in the details list, `<Button href={…}>` for
 * the three that are controls, and a bare `<a>` for anything that is neither. Both
 * components render an anchor, so the rules below apply to them exactly as they do to
 * the literal tag - and reading only one of the spellings is how this check would go
 * quiet without ever failing.
 */
const anchors = [...page.matchAll(/<(?:a|Button|Link)\b([^>]*)>/gsu)].map(
  ([, attributes]) => attributes ?? '',
);

describe('the invitation page', () => {
  it('renders outside the authenticated panel shell', () => {
    expect(rootLayout).toMatch(/\{:else if session\.isInvitation\}\s*\{@render children\(\)\}/u);
    expect(rootLayout.indexOf('{:else if session.isInvitation}')).toBeLessThan(
      rootLayout.indexOf('{:else if session.signedOut}'),
    );
    // The guard that keeps target resolution off this page. It no longer names the
    // inbox: every personal address is covered further down, and what has to hold here
    // is that an invitation never resolves a workspace out of the path.
    expect(rootLayout).toMatch(
      /if \(session\.isRootMode \|\| session\.isInvitation\) return;\s*const account = page\.params\.account/u,
    );
  });

  it('sends every link off the page to a new tab, without a referrer', () => {
    // These go to GitHub. `noreferrer` also implies `noopener`, which is the half that matters:
    // the token this page was reached with is in its URL, and an opener handle would hand the
    // destination a way back to the document holding it.
    const external = anchors.filter((attributes) => attributes.includes('githubProfile('));

    expect(external.length).toBeGreaterThan(1);
    for (const attributes of external) {
      expect(attributes, `external link is missing target: ${attributes}`).toMatch(
        /target="_blank"/u,
      );
      expect(attributes, `external link is missing rel: ${attributes}`).toMatch(
        /rel="[^"]*noreferrer/u,
      );
    }
  });

  it('keeps the sign-in links on this tab', () => {
    // Accepting, declining and opening the panel all have to land back somewhere that can act on
    // the answer, so the two rules above must not be applied to the whole page by reflex.
    const signIn = anchors.filter((attributes) => attributes.includes('signInUrl('));

    expect(signIn.length).toBeGreaterThanOrEqual(2);
    for (const attributes of signIn) {
      expect(attributes, `sign-in link should stay on this tab: ${attributes}`).not.toMatch(
        /target=/u,
      );
    }
  });

  it('answers a link that names no invitation the way every dead address is answered', () => {
    // 404 is terminal - there is nothing on the other end to press again for - while every other
    // failure is worth another go, so the branch is on the status rather than on there being an
    // error at all. What it reaches is the shared error card, which is the whole point: a link
    // that leads nowhere must not be told it was an invitation, because that describes something
    // the reader has no way to reach. The words themselves are pinned in panel-error.test.ts.
    expect(page).toMatch(/PanelApiError && error\.status === 404/u);

    const missing =
      /\{:else if failure !== null && failure\.missing\}([\s\S]*?)\{:else if/u.exec(page)?.[1] ??
      '';

    expect(missing).toMatch(/<ErrorCard\b/u);
    expect(missing, 'the not-found view should not offer a retry').not.toContain('<button');
    expect(missing.toLowerCase(), 'the not-found view should not name invitations').not.toContain(
      'invit',
    );
  });

  it('names the scope by more than its display name', () => {
    // "Smykla Skalski" identifies nothing on its own. The login is what a reader can check against
    // GitHub, and the kind says whether accepting joins an organisation or one person's
    // workspace. Both are optional in the payload, so both need a branch that copes without.
    expect(page).toContain('invitation.target_login');
    expect(page).toContain('invitation.target_kind');
    expect(page).toMatch(/invitation\.target_login === undefined/u);
    expect(page).toMatch(/invitation\.target_kind !== undefined/u);
  });
});

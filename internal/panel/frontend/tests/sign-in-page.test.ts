import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import { describeSessionEnd } from '../src/lib/panel-session';

/**
 * The front door. Everyone who signs out comes back to it, everyone whose session expires lands on
 * it, and it is the first page a person sees after installing the app - so what it says, and which
 * of its several states it says it in, is the whole of the first impression.
 */

const components = new URL('../src/components/', import.meta.url);
const read = (file: string): string => readFileSync(new URL(file, components), 'utf8');
const app = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8');
const session = readFileSync(new URL('../src/lib/panel-session.ts', import.meta.url), 'utf8');

describe('why the session ended', () => {
  it('does not tell someone who signed out that they were thrown out', () => {
    // The panel showed both under "Access revoked". Pressing Sign out is a thing you did.
    const out = describeSessionEnd({ code: 'signed_out', reason: 'You signed out' });

    expect(out.title).toBe('Signed out');
    expect(out.offersSignIn).toBe(true);
    expect(`${out.title} ${out.lead} ${out.note}`.toLowerCase()).not.toMatch(
      /revok|banned|removed/u,
    );
  });

  it('does not offer a way back that cannot work', () => {
    // Signing in again after being removed or banned reaches a 403 and nothing else. A button
    // that leads only to a refusal is worse than no button.
    for (const code of ['account_removed', 'account_banned']) {
      const notice = describeSessionEnd({ code, reason: '' });

      expect(notice.offersSignIn, code).toBe(false);
      expect(notice.note, code).toMatch(/ask whoever runs Smyklot/u);
    }
  });

  it('says something useful for a code it has never seen', () => {
    // The server can add one, and an old page must not answer with a blank card.
    const withReason = describeSessionEnd({ code: 'something_new', reason: 'The sky fell in' });
    const without = describeSessionEnd({ code: 'something_new', reason: '   ' });

    expect(withReason.lead).toBe('The sky fell in');
    expect(without.lead.length).toBeGreaterThan(0);
    for (const notice of [withReason, without]) {
      expect(notice.title.length).toBeGreaterThan(0);
      expect(notice.note.length).toBeGreaterThan(0);
      // Never assume the worst on no evidence.
      expect(notice.offersSignIn).toBe(true);
    }
  });

  it('covers every code the server can send', () => {
    // Read out of the Go source, so a new revocation reason cannot ship without words for it.
    const goSource = ['auth.go', 'root_access.go']
      .map((file) => readFileSync(new URL(`../../${file}`, import.meta.url), 'utf8'))
      .join('\n');
    const codes = [...goSource.matchAll(/revokeSession\([^,]+,\s*"?(?<code>[a-z_]+)"?/gu)]
      .map((match) => match.groups?.code ?? '')
      .filter((code) => code !== '' && code !== 'code');

    expect(codes.length).toBeGreaterThan(0);
    for (const code of codes) {
      expect(session, `${code} has no notice written for it`).toContain(`${code}:`);
    }
  });
});

describe('the page', () => {
  it('stands on the shared shell, at its own size', () => {
    // Same world as the invitation and the error pages; a smaller card, because it holds one
    // sentence and one button rather than a set of facts.
    expect(read('SignInPage.svelte')).toMatch(/<NightPage[^>]*\ssize="compact"/su);
  });

  it('replaces the panel rather than sitting inside it', () => {
    // It used to render as a plate in the workspace, beside a sidebar with nothing in it.
    expect(app).toMatch(/\{#if signedOut\}\s*<SignInPage/u);
    expect(app).not.toContain('SignedOut.svelte');
  });

  it('waits until the viewer is known before showing a front door', () => {
    // `viewer === null` is also true while the first request is in flight, and flashing a sign-in
    // page at someone who turns out to have a session is worse than a moment of skeleton.
    expect(app).toMatch(/const signedOut = \$derived\(!loading && viewer === null/u);
  });

  it('names the end of the session itself rather than racing the server for it', () => {
    // The socket closes as the sign-out lands, so whether the revocation event arrives is a race.
    // Losing it would greet someone as a stranger straight after they pressed Sign out.
    const signOut = /async function signOut\(\)[\s\S]*?\n {2}\}/u.exec(app)?.[0] ?? '';

    expect(signOut).toMatch(/sessionEnded = \{ code: 'signed_out'/u);
  });

  it('keeps the promise the sign-in copy makes', () => {
    // The page tells a reader GitHub is asked for their public profile and nothing else. That is
    // only true while the OAuth config asks for no scopes.
    const github = readFileSync(new URL('../../github.go', import.meta.url), 'utf8');
    const signIn = /func newGitHubSignIn[\s\S]*?\n\}/u.exec(github)?.[0] ?? '';

    expect(read('SignInPage.svelte')).toContain('public profile and nothing else');
    expect(signIn.length).toBeGreaterThan(0);
    expect(signIn, 'the sign-in config asks for scopes now').not.toMatch(
      /Scopes:\s*\[\]string\{"/u,
    );
  });
});

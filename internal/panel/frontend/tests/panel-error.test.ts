import { readFileSync, readdirSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import {
  describeFailure,
  readPanelFailure,
  type ErrorContent,
  type PanelFailure,
} from '../src/lib/panel-error';

/**
 * The error pages are written here, not in Go: the server sends a status and a code and this
 * decides what a person is told. These pin the two things nothing else can catch - that every
 * failure the server can actually produce has words written for it, and that a 404 keeps saying
 * only what a 404 can honestly say.
 */

const components = new URL('../src/components/', import.meta.url);
const read = (file: string): string => readFileSync(new URL(file, components), 'utf8');

function documentWith(content: string | null): Document {
  return {
    querySelector: () => (content === null ? null : { getAttribute: (): string => content }),
  } as unknown as Document;
}

describe('reading what the server left in the page', () => {
  it('finds nothing on a page the panel meant to serve', () => {
    // The overwhelmingly common case: the meta is present and empty.
    expect(readPanelFailure(documentWith(''))).toBeNull();
    expect(readPanelFailure(documentWith('   '))).toBeNull();
    expect(readPanelFailure(documentWith(null))).toBeNull();
  });

  it('reads a failure the server wrote', () => {
    const failure = readPanelFailure(
      documentWith('{"status":404,"code":"not_found","message":"panel route not found"}'),
    );

    expect(failure).toEqual({
      status: 404,
      code: 'not_found',
      message: 'panel route not found',
    });
  });

  it('treats anything it cannot read as no failure at all', () => {
    // Better to boot the panel than to show an error page built from nonsense.
    for (const content of ['__smyklot_panel_error__', '{', 'null', '[]', '{"code":"x"}']) {
      expect(readPanelFailure(documentWith(content)), content).toBeNull();
    }
  });
});

describe('what a reader is told', () => {
  /*
   * Every code the two navigable handlers can emit. A browser can only ever land on `serveAsset`
   * or the sign-in round trip, so this is the complete set of pages the server can produce - and
   * it is read out of the Go source rather than listed here, so adding a status there without
   * writing the words for it fails.
   */
  const goSource = ['auth.go', 'assets.go', 'invitations.go']
    .map((file) => readFileSync(new URL(`../../${file}`, import.meta.url), 'utf8'))
    .join('\n');

  const emitted = [
    ...goSource.matchAll(
      /http\.Status(?<status>[A-Za-z]+),\s*"(?<code>[a-z_]+)",\s*"(?<message>[^"]*)"/gu,
    ),
  ].map((match) => match.groups as { status: string; code: string; message: string });

  const STATUS_NUMBERS: Readonly<Record<string, number>> = {
    BadRequest: 400,
    Unauthorized: 401,
    Forbidden: 403,
    NotFound: 404,
    Conflict: 409,
    Gone: 410,
    InternalServerError: 500,
    BadGateway: 502,
  };

  it('finds the failures the server can produce', () => {
    // If this drops to nothing the sweep below is checking nothing.
    expect(emitted.length).toBeGreaterThan(8);
  });

  it.each(emitted.map((entry) => [`${entry.status} ${entry.code}`, entry] as const))(
    'has words for %s',
    (_label, entry) => {
      const status = STATUS_NUMBERS[entry.status];
      // A new status in the Go source lands here first, which is the point: it
      // cannot ship without someone deciding what the page says.
      expect(status, `${entry.status} is not a status this test knows`).toBeDefined();

      const content = describeFailure({
        status: status ?? 0,
        code: entry.code,
        message: entry.message,
      });

      expect(content.title.length).toBeGreaterThan(0);
      expect(content.lead.length).toBeGreaterThan(0);
      expect(content.note.length).toBeGreaterThan(0);
      // The server's own message is written for a developer reading a JSON body.
      expect(content.lead).not.toContain(entry.message);
      expect(content.note).not.toContain(entry.message);
    },
  );

  it('never mentions invitations in a 404', () => {
    // Bart's rule, and the reason for it: a reader holding a link that leads nowhere cannot act
    // on being told what the address would have been for. Every 404 falls through to the plain
    // one, whatever code came with it.
    const codes = ['not_found', 'invitation_not_found', '', 'something_new'];
    for (const code of codes) {
      const content = describeFailure({
        status: 404,
        code,
        message: 'this invitation does not exist',
      });

      expect(content, code).toEqual(
        describeFailure({ status: 404, code: 'not_found', message: '' }),
      );
      expect(`${content.title} ${content.lead} ${content.note}`.toLowerCase(), code).not.toContain(
        'invit',
      );
    }
  });

  it('offers a way on only where pressing it can work', () => {
    const action = (failure: PanelFailure): ErrorContent['action'] =>
      describeFailure(failure).action;

    // Sign-in stopped, so starting again is exactly the thing to do.
    expect(action({ status: 401, code: 'sign_in_failed', message: '' })?.kind).toBe('sign-in');
    expect(action({ status: 502, code: 'catalog_unavailable', message: '' })?.kind).toBe('sign-in');

    // An invitation that expired, was answered or names someone else is over. No
    // button on this page can change any of that, and one that looks like it might
    // is worse than none.
    for (const code of ['invitation_expired', 'wrong_identity', 'invalid_invitation']) {
      const status = code === 'invitation_expired' ? 410 : code === 'wrong_identity' ? 403 : 401;
      expect(action({ status, code, message: '' }), code).toBeNull();
    }
  });

  it('answers for a status nobody wrote a page for', () => {
    // A proxy or a future handler can produce anything; the reader still has to be
    // told whose problem it is.
    const client = describeFailure({ status: 418, code: 'teapot', message: '' });
    const server = describeFailure({ status: 599, code: 'unknown', message: '' });

    expect(client.status).toBe(418);
    expect(server.status).toBe(599);
    for (const content of [client, server]) {
      expect(content.lead.length).toBeGreaterThan(0);
      expect(content.note.length).toBeGreaterThan(0);
    }
    // 5xx is ours, 4xx is the address. The words have to differ or the split is
    // decorative.
    expect(client.lead).not.toBe(server.lead);
  });
});

describe('the pages that show it', () => {
  it('is one card, rendered by both pages', () => {
    // The invitation page's 404 used to be its own block with its own words. Sharing
    // the component is what stops the two drifting apart again.
    expect(read('ErrorPage.svelte')).toMatch(/<ErrorCard\b/u);
    expect(read('InvitationPage.svelte')).toMatch(/<ErrorCard\b/u);

    const holders = readdirSync(components)
      .filter((file) => file.endsWith('.svelte'))
      .filter((file) => read(file).includes('class="error-code"'));

    expect(holders).toEqual(['ErrorCard.svelte']);
  });

  it('carries the same head row as every other page out here', () => {
    // It went without a theme switch for a while and read as a page missing a piece. Whether the
    // switch is there is NightPage's business now; what matters here is that the error page does
    // not reach in and turn it off. Pinned in theme-switch.test.ts.
    expect(read('ErrorPage.svelte')).not.toContain('themeChoice');
  });

  it('keeps the number out of the reading order', () => {
    // The sentence under it says the same thing, and better.
    expect(read('ErrorCard.svelte')).toMatch(/class="error-code"\s+aria-hidden="true"/u);
  });
});

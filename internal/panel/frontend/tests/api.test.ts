import { describe, expect, it } from 'vitest';

import { PanelApiError, createPanelApi } from '../src/lib/api';

interface RecordedCall {
  url: string;
  init?: RequestInit;
}

function stubFetch(responses: Response[]): {
  calls: RecordedCall[];
  fetch: (input: string, init?: RequestInit) => Promise<Response>;
} {
  const calls: RecordedCall[] = [];
  const queue = [...responses];
  return {
    calls,
    fetch: (url: string, init?: RequestInit) => {
      calls.push({ url, init });
      const next = queue.shift();
      if (next === undefined) {
        throw new Error(`unexpected request to ${url}`);
      }
      return Promise.resolve(next);
    },
  };
}

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

const VIEWER = {
  account: {
    id: 'acc_1',
    provider: 'github',
    subject_id: '4242',
    login: 'ada',
    display_name: 'Ada Lovelace',
    avatar_url: null,
    first_seen_at: '2026-07-25T10:00:00Z',
    last_seen_at: '2026-07-25T11:00:00Z',
    can_pair: false,
  },
  is_owner: false,
};

describe('fetchViewer', () => {
  it('returns the signed-in account', async () => {
    const stub = stubFetch([jsonResponse(200, VIEWER)]);
    const api = createPanelApi('/panel', stub.fetch);

    await expect(api.fetchViewer()).resolves.toEqual(VIEWER);
    expect(stub.calls[0]?.url).toBe('/panel/api/me');
    expect(stub.calls[0]?.init?.credentials).toBe('same-origin');
  });

  // Arriving without a session is the ordinary first visit, so it must not
  // reach the page as an error banner.
  it('reports no session rather than failing', async () => {
    const stub = stubFetch([
      jsonResponse(401, { error: { code: 'unauthenticated', message: 'sign in first' } }),
    ]);
    const api = createPanelApi('/panel', stub.fetch);

    await expect(api.fetchViewer()).resolves.toBeNull();
  });

  it('surfaces any other failure with the panel error code', async () => {
    const stub = stubFetch([
      jsonResponse(500, { error: { code: 'storage', message: 'account store is unavailable' } }),
    ]);
    const api = createPanelApi('/panel', stub.fetch);

    await expect(api.fetchViewer()).rejects.toMatchObject({
      status: 500,
      code: 'storage',
      message: 'account store is unavailable',
    });
  });
});

describe('fetchAccounts', () => {
  it('unwraps the account list', async () => {
    const stub = stubFetch([jsonResponse(200, { accounts: [VIEWER.account] })]);
    const api = createPanelApi('/panel', stub.fetch);

    await expect(api.fetchAccounts()).resolves.toEqual([VIEWER.account]);
    expect(stub.calls[0]?.url).toBe('/panel/api/accounts');
  });

  it('rejects when the viewer is not the owner', async () => {
    const stub = stubFetch([
      jsonResponse(403, { error: { code: 'forbidden', message: 'owner only' } }),
    ]);
    const api = createPanelApi('/panel', stub.fetch);

    await expect(api.fetchAccounts()).rejects.toBeInstanceOf(PanelApiError);
  });
});

describe('signOut', () => {
  it('posts to the sign-out route', async () => {
    const stub = stubFetch([new Response(null, { status: 204 })]);
    const api = createPanelApi('/panel', stub.fetch);

    await api.signOut();

    expect(stub.calls[0]?.url).toBe('/panel/auth/signout');
    expect(stub.calls[0]?.init?.method).toBe('POST');
  });
});

// Every route behind the client is session-authenticated, so a request that
// went out without the cookie would read as being signed out rather than as a
// mistake.
describe('credentials', () => {
  it('are sent on every request the client makes', async () => {
    const stub = stubFetch([
      jsonResponse(200, VIEWER),
      jsonResponse(200, { accounts: [] }),
      new Response(null, { status: 204 }),
    ]);
    const api = createPanelApi('/panel', stub.fetch);

    await api.fetchViewer();
    await api.fetchAccounts();
    await api.signOut();

    expect(stub.calls).toHaveLength(3);
    for (const call of stub.calls) {
      expect(call.init?.credentials).toBe('same-origin');
    }
  });
});

describe('signInUrl', () => {
  it('points at the start route under the mount point', () => {
    expect(createPanelApi('/pairing', stubFetch([]).fetch).signInUrl()).toBe(
      '/pairing/auth/github/start',
    );
  });
});

// A reverse proxy or a crashed process can answer with something that is not
// the panel's envelope; the reader still needs to see that the call failed.
describe('non-envelope failures', () => {
  it('falls back to the status line', async () => {
    const stub = stubFetch([new Response('<html>502</html>', { status: 502 })]);
    const api = createPanelApi('/panel', stub.fetch);

    await expect(api.fetchAccounts()).rejects.toMatchObject({
      status: 502,
      code: 'unknown',
      message: 'panel request failed with status 502',
    });
  });
});

describe('setCanPair', () => {
  it('posts to approve or revoke and returns the updated account', async () => {
    const approved = { ...VIEWER.account, can_pair: true };
    const stub = stubFetch([jsonResponse(200, approved), jsonResponse(200, VIEWER.account)]);
    const api = createPanelApi('/panel', stub.fetch);

    await expect(api.setCanPair('acc_1', true)).resolves.toEqual(approved);
    expect(stub.calls[0]?.url).toBe('/panel/api/accounts/acc_1/approve');
    expect(stub.calls[0]?.init?.method).toBe('POST');

    await api.setCanPair('acc_1', false);
    expect(stub.calls[1]?.url).toBe('/panel/api/accounts/acc_1/revoke');
  });

  // An id is server-generated, but building a path by concatenation is how a
  // future id containing a slash would silently address another route.
  it('escapes the account id into the path', async () => {
    const stub = stubFetch([jsonResponse(200, VIEWER.account)]);
    const api = createPanelApi('/panel', stub.fetch);

    await api.setCanPair('a/b', true);

    expect(stub.calls[0]?.url).toBe('/panel/api/accounts/a%2Fb/approve');
  });

  // `encodeURIComponent` leaves the dot alone, so this is the one that would
  // survive as a segment and get resolved away into the route above.
  it('escapes a dotted account id so it cannot climb a level', async () => {
    const stub = stubFetch([jsonResponse(200, VIEWER.account)]);
    const api = createPanelApi('/panel', stub.fetch);

    await api.setCanPair('..', true);

    expect(stub.calls[0]?.url).toBe('/panel/api/accounts/%2E%2E/approve');
  });

  it('surfaces a refusal', async () => {
    const stub = stubFetch([
      jsonResponse(403, { error: { code: 'forbidden', message: 'owner only' } }),
    ]);
    const api = createPanelApi('/panel', stub.fetch);

    await expect(api.setCanPair('acc_1', true)).rejects.toMatchObject({
      status: 403,
      code: 'forbidden',
    });
  });
});

describe('createPairLink', () => {
  it('returns the link the daemon minted', async () => {
    const link = {
      pairing_id: 'pair-1',
      role: 'operator',
      scopes: ['read', 'write'],
      expires_at: '2026-07-25T10:10:00Z',
      pairing_url: 'harness://pair?payload=abc',
    };
    const stub = stubFetch([jsonResponse(200, link)]);
    const api = createPanelApi('/panel', stub.fetch);

    await expect(api.createPairLink()).resolves.toEqual(link);
    expect(stub.calls[0]?.url).toBe('/panel/api/pair-links');
    expect(stub.calls[0]?.init?.method).toBe('POST');
  });

  // An account the owner has not approved gets a 403, and the page has to show
  // the panel's own sentence rather than a generic failure.
  it('surfaces the reason an unapproved account is refused', async () => {
    const stub = stubFetch([
      jsonResponse(403, {
        error: { code: 'forbidden', message: 'the panel owner has not allowed this account' },
      }),
    ]);
    const api = createPanelApi('/panel', stub.fetch);

    await expect(api.createPairLink()).rejects.toMatchObject({
      status: 403,
      message: 'the panel owner has not allowed this account',
    });
  });

  it('surfaces a panel that has not paired with the daemon', async () => {
    const stub = stubFetch([
      jsonResponse(503, {
        error: { code: 'unavailable', message: 'the panel has not paired with the daemon yet' },
      }),
    ]);
    const api = createPanelApi('/panel', stub.fetch);

    await expect(api.createPairLink()).rejects.toMatchObject({
      status: 503,
      code: 'unavailable',
    });
  });
});

describe('fetchPairings', () => {
  const pairing = {
    pairing_id: 'pair-1',
    state: 'active',
    role: 'operator',
    created_at: '2026-07-26T10:00:00Z',
    expires_at: '2026-07-26T10:10:00Z',
    account_id: 'acc_1',
  };

  it('returns the pairing list and the daemon that answered', async () => {
    const stub = stubFetch([jsonResponse(200, { daemon_version: '52.0.0', pairings: [pairing] })]);
    const api = createPanelApi('/panel', stub.fetch);

    await expect(api.fetchPairings()).resolves.toEqual({
      daemon_version: '52.0.0',
      pairings: [pairing],
    });
    expect(stub.calls[0]?.url).toBe('/panel/api/pairings');
  });

  // A daemon older than the field answers the route without it, and the
  // pairings are the point of the call.
  it('reads a list from a daemon that reports no version', async () => {
    const stub = stubFetch([jsonResponse(200, { pairings: [pairing] })]);
    const api = createPanelApi('/panel', stub.fetch);

    await expect(api.fetchPairings()).resolves.toEqual({ pairings: [pairing] });
  });

  it('surfaces a daemon the panel cannot reach', async () => {
    const stub = stubFetch([
      jsonResponse(503, {
        error: { code: 'unavailable', message: 'the panel has not paired with the daemon yet' },
      }),
    ]);
    const api = createPanelApi('/panel', stub.fetch);

    await expect(api.fetchPairings()).rejects.toMatchObject({ status: 503 });
  });
});

describe('revokePairing', () => {
  it('posts to the revoke route and returns what it did', async () => {
    const revoked = {
      pairing_id: 'pair-1',
      outcome: 'device_revoked',
      revoked_at: '2026-07-26T11:00:00Z',
    };
    const stub = stubFetch([jsonResponse(200, revoked)]);
    const api = createPanelApi('/panel', stub.fetch);

    await expect(api.revokePairing('pair-1')).resolves.toEqual(revoked);
    expect(stub.calls[0]?.url).toBe('/panel/api/pairings/pair-1/revoke');
    expect(stub.calls[0]?.init?.method).toBe('POST');
  });

  // The id is the daemon's, but building a path by concatenation is how one
  // containing a slash would silently address another route.
  it('escapes the pairing id into the path', async () => {
    const stub = stubFetch([
      jsonResponse(200, { pairing_id: 'a/b', outcome: 'link_withdrawn', revoked_at: 'now' }),
    ]);
    const api = createPanelApi('/panel', stub.fetch);

    await api.revokePairing('a/b');

    expect(stub.calls[0]?.url).toBe('/panel/api/pairings/a%2Fb/revoke');
  });

  // The dot is the one `encodeURIComponent` passes through, so `..` would reach
  // the request as a real segment and be resolved away, sending a revoke to
  // whatever sits one level up. The Rust client escapes it for the same reason.
  it('escapes a dotted pairing id so it cannot climb a level', async () => {
    const stub = stubFetch([
      jsonResponse(200, { pairing_id: '..', outcome: 'link_withdrawn', revoked_at: 'now' }),
    ]);
    const api = createPanelApi('/panel', stub.fetch);

    await api.revokePairing('..');

    expect(stub.calls[0]?.url).toBe('/panel/api/pairings/%2E%2E/revoke');
  });

  // The panel answers this for a pairing that belongs to somebody else and for
  // one that does not exist, deliberately alike, and the page shows its
  // sentence rather than guessing which it was.
  it('surfaces a pairing that is not the viewer to withdraw', async () => {
    const stub = stubFetch([
      jsonResponse(403, {
        error: {
          code: 'forbidden',
          message: 'no pairing with that id is available to this account',
        },
      }),
    ]);
    const api = createPanelApi('/panel', stub.fetch);

    await expect(api.revokePairing('pair-1')).rejects.toMatchObject({
      status: 403,
      message: 'no pairing with that id is available to this account',
    });
  });
});

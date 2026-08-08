/**
 * A local mock backend for the panel, used only in development.
 *
 * The real panel is a Rust binary that fronts the daemon, GitHub OAuth, and a
 * panel-to-daemon token. Standing any of that up to move a control is
 * unreasonable, so this serves canned data for the panel's own HTTP routes and
 * the live-update socket, letting the whole Svelte app be driven by hand.
 *
 * It is a Vite plugin that no-ops unless `HARNESS_PANEL_DEV_MOCK` is `1`, so the
 * default dev server, the build, and the tests are untouched. All state lives in
 * memory for the lifetime of one server process: restarting resets it, which is
 * the point of a fixture.
 *
 * The shapes returned here match what `src/lib/types.ts` declares and what the
 * Rust handlers in `src/http.rs` emit, so the app cannot tell it apart from the
 * real backend by its answers - only by how forgettable it was to start.
 */

import { randomUUID } from 'node:crypto';
import type { Server as HttpServer, IncomingMessage, ServerResponse } from 'node:http';
import { WebSocket, WebSocketServer } from 'ws';
import type { Connect, Plugin } from 'vite';

/**
 * Vite types its HTTP server as a union that also admits HTTP/2 variants. Its
 * dev and preview servers are always plain `node:http` servers, so this narrows
 * the union at the one place that needs the `upgrade` event and the socket the
 * WebSocket upgrade hands back, both of which are HTTP/1-only.
 */
type DevHttpServer = HttpServer;

/**
 * The root the mock matches routes against. When the mock is enabled the panel
 * is the whole dev server: the plugin overrides Vite's `base` to `/` and
 * rewrites the `harness-panel-base` meta tag to match, so the app builds its
 * API URLs at `/api/...` and the sentinel never appears.
 */
const BASE = '';

/** How long a minted link stays claimable. Short enough to watch the gauge drain. */
const LINK_TTL_MS = 10 * 60_000;

/** Whether the mock should take over the dev server. */
function enabled(): boolean {
  return process.env.HARNESS_PANEL_DEV_MOCK === '1';
}

/**
 * A failure the browser is allowed to see, mirroring `ApiError` in `error.rs`.
 *
 * Every variant the real backend exposes carries a stable machine-readable code,
 * so the app can tell authentication and sign-in failures from an internal one
 * without matching on prose. The mock answers the two a developer is likely to
 * reach by typing a bad id: a missing account is a `404 not_found`, and a
 * pairing the viewer cannot reach is a `403 forbidden` - the same answer the
 * real backend gives, so the page shows its sentence rather than a stack trace.
 */
class MockApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = 'MockApiError';
  }
}

/** The in-memory state one server process owns. Restarting drops all of it. */
interface MockState {
  signedIn: boolean;
  /** Which account the current session is signed in as. Drives `/api/me`. */
  viewerId: string;
  accounts: Account[];
  pairings: Pairing[];
  sockets: Set<WebSocket>;
}

/** Only the fields the app reads. Mirrors `PanelAccount` in `types.ts`. */
interface Account {
  id: string;
  provider: string;
  subject_id: string;
  login: string;
  display_name: string;
  avatar_url: string | null;
  first_seen_at: string;
  last_seen_at: string;
  can_pair: boolean;
}

/** Mirrors `PairLink` in `types.ts`. */
interface PairLink {
  pairing_id: string;
  role: string;
  scopes: string[];
  expires_at: string;
  pairing_url: string;
}

/** Only the required fields plus the ones a canned row carries. Mirrors `PanelPairing`. */
interface Pairing {
  pairing_id: string;
  state: string;
  role: string;
  created_at: string;
  expires_at: string;
  claimed_at?: string;
  revoked_at?: string;
  device?: { client_id: string; display_name: string; platform: string; last_seen_at?: string };
  account_id?: string;
}

/** The owner, who sees the roster and can approve others. */
const OWNER_ID = 'acc_owner';

function seed(): MockState {
  const now = Date.now();
  const iso = (offsetMs: number): string => new Date(now + offsetMs).toISOString();
  const owner: Account = {
    id: OWNER_ID,
    provider: 'github',
    subject_id: '1001',
    login: 'ada',
    display_name: 'Ada Lovelace',
    avatar_url: null,
    first_seen_at: iso(-86_400_000),
    last_seen_at: iso(-3_600_000),
    can_pair: true,
  };
  const approvedPeer: Account = {
    id: 'acc_grace',
    provider: 'github',
    subject_id: '1002',
    login: 'grace',
    display_name: 'Grace Hopper',
    avatar_url: null,
    first_seen_at: iso(-43_200_000),
    last_seen_at: iso(-7_200_000),
    can_pair: true,
  };
  const pendingPeer: Account = {
    id: 'acc_alan',
    provider: 'github',
    subject_id: '1003',
    login: 'alan',
    display_name: 'Alan Turing',
    avatar_url: null,
    first_seen_at: iso(-21_600_000),
    last_seen_at: iso(-1_800_000),
    can_pair: false,
  };
  const pairings: Pairing[] = [
    {
      pairing_id: 'pair_active_owner',
      state: 'active',
      role: 'operator',
      created_at: iso(-172_800_000),
      expires_at: iso(86_400_000),
      claimed_at: iso(-170_000_000),
      account_id: OWNER_ID,
      device: {
        client_id: 'dev_laptop',
        display_name: 'Ada’s MacBook Pro',
        platform: 'macOS',
        last_seen_at: iso(-600_000),
      },
    },
    {
      pairing_id: 'pair_claimed_grace',
      state: 'claimed',
      role: 'operator',
      created_at: iso(-86_400_000),
      expires_at: iso(86_400_000),
      claimed_at: iso(-80_000_000),
      account_id: 'acc_grace',
      device: {
        client_id: 'dev_grace_workstation',
        display_name: 'Grace’s workstation',
        platform: 'linux',
      },
    },
    {
      pairing_id: 'pair_expired_owner',
      state: 'expired',
      role: 'operator',
      created_at: iso(-345_600_000),
      expires_at: iso(-285_600_000),
      account_id: OWNER_ID,
    },
    {
      pairing_id: 'pair_revoked_alan',
      state: 'revoked',
      role: 'operator',
      created_at: iso(-259_200_000),
      expires_at: iso(-199_200_000),
      revoked_at: iso(-200_000_000),
      account_id: 'acc_alan',
    },
  ];
  return {
    signedIn: true,
    viewerId: OWNER_ID,
    accounts: [owner, approvedPeer, pendingPeer],
    pairings,
    sockets: new Set(),
  };
}

/** The signed-in viewer, packaged as the app expects. */
function viewer(state: MockState): { account: Account; is_owner: boolean } {
  const account = state.accounts.find((entry) => entry.id === state.viewerId);
  if (account === undefined) {
    throw new Error(`mock viewer account ${state.viewerId} is missing from the seed`);
  }
  return { account, is_owner: state.viewerId === OWNER_ID };
}

/** Send a pairing change to every connected socket, matching the frame `events.ts` reads. */
function broadcast(state: MockState, change: string, pairing: Pairing): void {
  const frame = JSON.stringify({ type: 'pairing', change, pairing });
  for (const socket of state.sockets) {
    if (socket.readyState === WebSocket.OPEN) {
      socket.send(frame);
    }
  }
}

/** A stable base path prefix so route matching reads like the Rust table in `http.rs`. */
function route(path: string): string {
  return `${BASE}${path}`;
}

/**
 * The Vite plugin. Overrides the base and opens the browser when enabled, and
 * installs the mock middleware and the socket upgrade handler on the server.
 */
export function mockServer(): Plugin {
  return {
    name: 'harness-panel-mock-server',
    config() {
      if (!enabled()) {
        return;
      }
      // The panel is the whole dev server when the mock is on, so it mounts at
      // the root: the sentinel `base` is dropped, the dev URL is just the host,
      // and the browser opens there on start.
      return {
        base: '/',
        server: { open: '/' },
      };
    },
    transformIndexHtml(html) {
      if (!enabled()) {
        return html;
      }
      // The app reads its mount point from this meta tag, so it has to agree
      // with the root `base` above or every API URL would still carry the
      // sentinel and miss the mock's routes.
      return html.replace(
        /name="harness-panel-base" content="\/__harness_panel_base__"/,
        'name="harness-panel-base" content="/"',
      );
    },
    configureServer(server) {
      if (!enabled()) {
        return;
      }
      install(server.httpServer as DevHttpServer, server.middlewares);
    },
    configurePreviewServer(server) {
      if (!enabled()) {
        return;
      }
      install(server.httpServer as DevHttpServer, server.middlewares);
    },
  };
}

/**
 * Attach the mock to one HTTP server.
 *
 * The middleware answers the panel's own routes and lets everything else fall
 * through to Vite. The upgrade handler claims only the panel socket path, so
 * Vite's own HMR socket is left alone.
 */
function install(httpServer: DevHttpServer | undefined, middlewares: Connect.Server): void {
  if (httpServer === undefined) {
    throw new Error('the mock needs the dev server HTTP server, which was not provided');
  }
  const state = seed();
  const wss = new WebSocketServer({ noServer: true });

  httpServer.on('upgrade', (request, socket, head) => {
    if (request.url !== route('/api/ws')) {
      return;
    }
    wss.handleUpgrade(request, socket, head, (ws) => {
      const typed = ws as WebSocket;
      state.sockets.add(typed);
      // Nothing is sent on connect, matching the real backend: it opens the
      // socket and waits for changes. `events.ts` re-reads on the open event
      // itself, so the page refreshes its list the way it would against the
      // panel, without a frame the panel never sends.
      typed.on('close', () => {
        state.sockets.delete(typed);
      });
    });
  });

  middlewares.use((req, res, next) => handle(state, req, res, next));
}

/**
 * Route one request, or hand it back to Vite.
 *
 * Paths are matched against the panel base, matching what the app asks for. The
 * real panel answers these in `src/http.rs`; this returns the same shapes so the
 * app renders every state without a backend.
 */
function handle(
  state: MockState,
  req: IncomingMessage,
  res: ServerResponse,
  next: Connect.NextFunction,
): void {
  const url = req.url ?? '';
  const method = req.method ?? 'GET';

  // The sentinel is not a real path when the mock is on - the panel mounts at
  // root - so refuse it rather than letting Vite's SPA fallback serve the app
  // there too. A developer who has it bookmarked from the old setup sees a 404
  // instead of a page whose API calls all miss the mock's root-level routes.
  if (url.startsWith('/__harness_panel_base__')) {
    respond(res, 404, {
      error: { code: 'not_found', message: 'the panel is mounted at / in dev mock mode' },
    });
    return;
  }

  // Auth: the mock pretends GitHub already said yes. The start route accepts an
  // optional `?as=<login>` so a developer can sign in as a non-owner to exercise
  // the awaiting-approval state, and redirects straight to the callback, which
  // redirects back to the app root.
  if (url.startsWith(route('/auth/github/start')) && method === 'GET') {
    const as = new URL(url, 'http://localhost').searchParams.get('as') ?? '';
    const query = as === '' ? '' : `?${new URLSearchParams({ as })}`;
    res.writeHead(302, { Location: `${route('/auth/github/callback')}${query}` });
    res.end();
    return;
  }
  if (url.startsWith(route('/auth/github/callback')) && method === 'GET') {
    const as = new URL(url, 'http://localhost').searchParams.get('as') ?? '';
    const account = as === '' ? undefined : state.accounts.find((entry) => entry.login === as);
    state.viewerId = account?.id ?? OWNER_ID;
    state.signedIn = true;
    res.writeHead(302, { Location: `${BASE}/` });
    res.end();
    return;
  }
  if (url === route('/auth/signout') && method === 'POST') {
    state.signedIn = false;
    respond(res, 204, null);
    return;
  }

  // Everything below is session-authenticated. A signed-out viewer gets the
  // same `401` the real backend returns - code `unauthenticated` and the panel's
  // own sentence - which the app reads as "show the sign-in page".
  if (!state.signedIn) {
    respond(res, 401, {
      error: { code: 'unauthenticated', message: 'sign in to use the panel' },
    });
    return;
  }

  // Every authenticated route below can throw `MockApiError` - a missing
  // record, a non-owner reaching an owner-only route, or an account that
  // cannot pair. The real backend answers each with a JSON envelope, so the
  // throw is caught here rather than let loose on Vite as an HTML 500.
  try {
    if (url === route('/api/me') && method === 'GET') {
      respond(res, 200, viewer(state));
      return;
    }
    if (url === route('/api/accounts') && method === 'GET') {
      requireOwner(state);
      respond(res, 200, { accounts: state.accounts });
      return;
    }
    if (url === route('/api/pairings') && method === 'GET') {
      const current = viewer(state);
      const visible = state.pairings.filter(
        (pairing) => current.is_owner || pairing.account_id === current.account.id,
      );
      respond(res, 200, { pairings: visible, daemon_version: 'harness-daemon dev-mock' });
      return;
    }
    if (url === route('/api/pair-links') && method === 'POST') {
      const link = mint(state);
      respond(res, 200, link);
      return;
    }

    const approve = url.match(paramRoute('/api/accounts', 'approve'));
    const revokeAccount = url.match(paramRoute('/api/accounts', 'revoke'));
    const revokePairing = url.match(paramRoute('/api/pairings', 'revoke'));
    if (approve && method === 'POST') {
      requireOwner(state);
      const account = setCanPair(state, approve.groups?.id ?? '', true);
      respond(res, 200, account);
      return;
    }
    if (revokeAccount && method === 'POST') {
      requireOwner(state);
      const account = setCanPair(state, revokeAccount.groups?.id ?? '', false);
      respond(res, 200, account);
      return;
    }
    if (revokePairing && method === 'POST') {
      const outcome = revoke(state, revokePairing.groups?.id ?? '');
      respond(res, 200, outcome);
      return;
    }
  } catch (error) {
    if (error instanceof MockApiError) {
      respond(res, error.status, {
        error: { code: error.code, message: error.message },
      });
      return;
    }
    // A throw the mock did not expect is its own bug. The real backend answers
    // `internal` and logs the cause; the mock does the same, and the cause lands
    // in the terminal rather than only in the browser.
    console.error('panel mock handler threw:', error);
    respond(res, 500, {
      error: { code: 'internal', message: 'the panel could not complete this request' },
    });
    return;
  }

  next();
}

/**
 * Match a parameterised route under the panel base, capturing the id segment.
 *
 * The base is escaped so its slashes are literal, and the id segment refuses a
 * slash so it cannot reach past the route's own level - the same reason the app's
 * `pathSegment` encodes dots before joining a path.
 */
function paramRoute(path: string, action: string): RegExp {
  const escaped = route(path).replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  return new RegExp(`^${escaped}/(?<id>[^/]+)/${action}$`);
}

/** Mint a link for the signed-in account and announce it to every watcher. */
function mint(state: MockState): PairLink {
  const current = viewer(state);
  if (!current.account.can_pair) {
    throw new MockApiError(
      403,
      'forbidden',
      'the panel owner has not allowed this account to generate pairing links',
    );
  }
  const now = Date.now();
  const pairingId = `pair_${randomUUID()}`;
  const expiresAt = new Date(now + LINK_TTL_MS).toISOString();
  const pairing: Pairing = {
    pairing_id: pairingId,
    state: 'pending',
    role: 'operator',
    created_at: new Date(now).toISOString(),
    expires_at: expiresAt,
    account_id: current.account.id,
  };
  state.pairings.push(pairing);
  broadcast(state, 'minted', pairing);
  return {
    pairing_id: pairingId,
    role: pairing.role,
    scopes: ['pair:device'],
    expires_at: expiresAt,
    pairing_url: `harness://pair?payload=${pairingId}`,
  };
}

/** Refuse the call when the viewer is not the owner, matching the real backend. */
function requireOwner(state: MockState): void {
  if (state.viewerId !== OWNER_ID) {
    throw new MockApiError(403, 'forbidden', 'only the panel owner can do that');
  }
}

/** Toggle an account's ability to pair and return the updated record. */
function setCanPair(state: MockState, id: string, granted: boolean): Account {
  const account = state.accounts.find((entry) => entry.id === id);
  if (account === undefined) {
    throw new MockApiError(404, 'not_found', 'no such account');
  }
  account.can_pair = granted;
  return account;
}

/** Mark a pairing revoked and announce it. Non-owners may only revoke their own. */
function revoke(
  state: MockState,
  id: string,
): { pairing_id: string; outcome: string; revoked_at: string } {
  const current = viewer(state);
  const pairing = state.pairings.find((entry) => entry.pairing_id === id);
  if (pairing === undefined) {
    throw new MockApiError(
      403,
      'forbidden',
      'no pairing with that id is available to this account',
    );
  }
  if (!current.is_owner && pairing.account_id !== current.account.id) {
    throw new MockApiError(
      403,
      'forbidden',
      'no pairing with that id is available to this account',
    );
  }
  pairing.state = 'revoked';
  pairing.revoked_at = new Date().toISOString();
  broadcast(state, 'revoked', pairing);
  return {
    pairing_id: id,
    outcome: pairing.claimed_at === undefined ? 'link_withdrawn' : 'device_revoked',
    revoked_at: pairing.revoked_at,
  };
}

/**
 * Write a response.
 *
 * A `null` body is a no-content response; an object is JSON. The status is set
 * on the response so `writeHead` is only used for redirects, which carry headers.
 */
function respond(res: ServerResponse, status: number, body: unknown): void {
  if (body === null) {
    res.statusCode = status;
    res.end();
    return;
  }
  res.statusCode = status;
  res.setHeader('Content-Type', 'application/json');
  res.end(JSON.stringify(body));
}

export default mockServer;

import { panelUrl } from './base';
import type { PanelSocketFactory, PanelStreamHandlers } from './events';
import { openPanelStream, panelStreamUrl } from './events';
import type {
  PairLink,
  PairingRevoke,
  PanelAccount,
  PanelErrorBody,
  PanelPairings,
  PanelViewer,
} from './types';

/** A panel response the caller cannot treat as data. */
export class PanelApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = 'PanelApiError';
  }
}

export interface PanelApi {
  /** The signed-in person, or `null` when the session is absent or expired. */
  fetchViewer(): Promise<PanelViewer | null>;
  /** Everyone who has signed in. Owner only; throws 403 for anyone else. */
  fetchAccounts(): Promise<PanelAccount[]>;
  signOut(): Promise<void>;
  signInUrl(): string;
  /** Owner only. Grants or withdraws an account's ability to pair. */
  setCanPair(accountId: string, granted: boolean): Promise<PanelAccount>;
  /** Mint a link for the signed-in account. The reply is shown once. */
  createPairLink(): Promise<PairLink>;
  /** The viewer's own pairings, or everyone's for the owner. */
  fetchPairings(): Promise<PanelPairings>;
  /** Cut off a device, or withdraw a link nobody claimed. */
  revokePairing(pairingId: string): Promise<PairingRevoke>;
  /**
   * Hold a socket open for what the panel learns without being asked. Returns
   * the function that closes it again.
   */
  openStream(handlers: PanelStreamHandlers): () => void;
}

type FetchLike = (input: string, init?: RequestInit) => Promise<Response>;

/**
 * The socket is built here rather than taken as a required argument, because
 * only a test ever wants a different one and every caller in the app wants this.
 */
const browserSocket: PanelSocketFactory = (url) => new WebSocket(url);

export function createPanelApi(
  base: string,
  fetchImpl: FetchLike,
  createSocket: PanelSocketFactory = browserSocket,
): PanelApi {
  const request = async (path: string, init?: RequestInit): Promise<Response> => {
    // `credentials` goes last so a caller's init cannot displace it. Every
    // route behind this helper is session-authenticated, and dropping the
    // cookie would read as being signed out rather than as a mistake.
    const response = await fetchImpl(panelUrl(base, path), {
      ...init,
      credentials: 'same-origin',
    });
    if (!response.ok) {
      throw await readError(response);
    }
    return response;
  };

  return {
    async fetchViewer(): Promise<PanelViewer | null> {
      const response = await fetchImpl(panelUrl(base, '/api/me'), {
        credentials: 'same-origin',
      });
      // Signing in is the whole point of the page, so "no session" is the
      // expected first state rather than a failure worth surfacing.
      if (response.status === 401) {
        return null;
      }
      if (!response.ok) {
        throw await readError(response);
      }
      return (await response.json()) as PanelViewer;
    },

    async fetchAccounts(): Promise<PanelAccount[]> {
      const response = await request('/api/accounts');
      const body = (await response.json()) as { accounts: PanelAccount[] };
      return body.accounts;
    },

    async signOut(): Promise<void> {
      await request('/auth/signout', { method: 'POST' });
    },

    signInUrl(): string {
      return panelUrl(base, '/auth/github/start');
    },

    async setCanPair(accountId: string, granted: boolean): Promise<PanelAccount> {
      const action = granted ? 'approve' : 'revoke';
      const response = await request(`/api/accounts/${pathSegment(accountId)}/${action}`, {
        method: 'POST',
      });
      return (await response.json()) as PanelAccount;
    },

    async createPairLink(): Promise<PairLink> {
      const response = await request('/api/pair-links', { method: 'POST' });
      return (await response.json()) as PairLink;
    },

    async fetchPairings(): Promise<PanelPairings> {
      const response = await request('/api/pairings');
      return (await response.json()) as PanelPairings;
    },

    async revokePairing(pairingId: string): Promise<PairingRevoke> {
      const response = await request(`/api/pairings/${pathSegment(pairingId)}/revoke`, {
        method: 'POST',
      });
      return (await response.json()) as PairingRevoke;
    },

    openStream(handlers: PanelStreamHandlers): () => void {
      // Resolved here rather than when the api is built, so a page that never
      // signs anybody in never reads the location at all.
      return openPanelStream(panelStreamUrl(base, window.location.href), handlers, createSocket);
    },
  };
}

/**
 * Encode one id into one path segment.
 *
 * `encodeURIComponent` alone is not enough: it leaves `.` untouched, so an id
 * of `..` survives as a segment that URL resolution then removes, addressing
 * the route above the intended one. Ids are server-generated and none looks
 * like this today, but building a path by concatenation is exactly how that
 * stops being true without anybody noticing.
 *
 * The panel's Rust client escapes the dot for the same reason. The two do not
 * produce byte-identical output — this one leaves sub-delimiters that resolve
 * to themselves — but they agree on everything that must not survive as a
 * segment of its own.
 */
function pathSegment(value: string): string {
  // Safe after encoding rather than before: a percent escape is `%` and two
  // hex digits, so it can never contain the dot this puts back.
  return encodeURIComponent(value).replace(/\./g, '%2E');
}

async function readError(response: Response): Promise<PanelApiError> {
  let code = 'unknown';
  let message = `panel request failed with status ${response.status}`;
  try {
    const body = (await response.json()) as Partial<PanelErrorBody>;
    if (body.error?.code !== undefined) {
      code = body.error.code;
    }
    if (body.error?.message !== undefined && body.error.message !== '') {
      message = body.error.message;
    }
  } catch {
    // A proxy or a crash can answer with something that is not the panel's
    // error envelope; the status line is still worth reporting.
  }
  return new PanelApiError(response.status, code, message);
}

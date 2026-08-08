/** One person who has signed in to the panel at least once. */
export interface PanelAccount {
  id: string;
  provider: string;
  subject_id: string;
  login: string;
  display_name: string;
  avatar_url: string | null;
  first_seen_at: string;
  last_seen_at: string;
  /** Whether the owner has allowed this account to generate pairing links. */
  can_pair: boolean;
}

/** A link the daemon minted, shown once and never stored. */
export interface PairLink {
  pairing_id: string;
  role: string;
  scopes: string[];
  expires_at: string;
  pairing_url: string;
}

/** The device a claimed link became. */
export interface PanelPairingDevice {
  client_id: string;
  display_name: string;
  platform: string;
  /** Absent until the device makes its first authenticated request. */
  last_seen_at?: string;
  revoked_at?: string;
}

/** One link the panel minted, and what became of it. */
export interface PanelPairing {
  pairing_id: string;
  /**
   * `pending`, `claimed`, `active`, `expired`, or `revoked`, as the daemon
   * reports it. Deliberately a string rather than a union: the daemon owns this
   * vocabulary, and a state it grows should reach the page as itself instead of
   * failing a type the panel would have to be rebuilt to widen.
   */
  state: string;
  role: string;
  created_at: string;
  expires_at: string;
  claimed_at?: string;
  revoked_at?: string;
  device?: PanelPairingDevice;
  /**
   * The account the link was minted for. Absent for one the panel has no record
   * of, which only the owner is shown at all.
   */
  account_id?: string;
}

/** The pairing list, and what the daemon said about itself while answering. */
export interface PanelPairings {
  pairings: PanelPairing[];
  /**
   * Absent from a daemon older than the field, and from any answer the panel
   * could not get, so the footer omits the mark rather than inventing one.
   */
  daemon_version?: string;
}

/** What an unpair did, as the daemon reports it. */
export interface PairingRevoke {
  pairing_id: string;
  /** `device_revoked`, `link_withdrawn`, or `already_revoked`. */
  outcome: string;
  revoked_at: string;
}

/** The signed-in person, plus what the panel lets them see. */
export interface PanelViewer {
  account: PanelAccount;
  is_owner: boolean;
}

/** Error envelope every panel route uses for a non-2xx response. */
export interface PanelErrorBody {
  error: {
    code: string;
    message: string;
  };
}

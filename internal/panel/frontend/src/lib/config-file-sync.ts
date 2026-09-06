import type { JsonValue } from './merge';

export type ConfigFileSyncState = 'off' | 'pending' | 'ready' | 'proposed' | 'blocked';
export type ConfigFileChoiceSide = 'panel' | 'file';

/** Cached observation: a pending state can retain an older check for context. */
export interface ConfigFileSyncStatus {
  enabled: boolean;
  available: boolean;
  status: ConfigFileSyncState;
  last_check?: {
    checked_at: string;
    status: ConfigFileSyncState;
    settings_current: boolean;
    head?: string;
    path?: string;
    problem?: string;
    message?: string;
    conflict_count?: number;
    conflict_paths?: string[][];
    proposal?: { number: number; url: string };
  };
}

/** Each document preserves independent edits while choosing overlapping values. */
export interface ConfigFilePreviewChoice {
  side: ConfigFileChoiceSide;
  available: boolean;
  document?: JsonValue;
  import_panel: boolean;
  publish_file: boolean;
  message?: string;
}

export interface ConfigFilePreview {
  status: ConfigFileSyncState;
  checked_at: string;
  path?: string;
  head?: string;
  problem?: string;
  message?: string;
  conflict_count?: number;
  conflict_paths?: string[][];
  review_token?: string;
  choices?: ConfigFilePreviewChoice[];
}

/** Only a fresh comparison token and a choice cross the write boundary. */
export interface ConfigFileResolutionInput {
  review_token: string;
  side: ConfigFileChoiceSide;
}

export interface ConfigFileResolutionReceipt {
  status: 'pending';
}

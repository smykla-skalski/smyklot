import { createContext } from 'svelte';

import type { FindEntry } from './components/FindPalette.svelte';

/**
 * What the finder can reach, offered to whoever else needs it.
 *
 * The palette and the search results page ask the same question and must not answer it
 * differently: a reader who presses "see all" is asking to see the rest of what the
 * palette was already showing them, not the output of a second search written twice.
 * The layout knows what the reader can reach - which console is open, which workspace,
 * which pages exist in it - so it composes this once and hands it down.
 */
export interface Finder {
  /** Everything reachable without asking the service: pages, workspaces, the inbox. */
  entries: readonly FindEntry[];
  /** Repositories and people, which only the service knows. */
  lookup?: (query: string) => Promise<FindEntry[]>;
  /** What the other console is called, for the rows that leave this one. */
  crossLabel: string;
  /** Whose results these are: the workspace's name, or the console's. */
  scopeName: string;
}

export const [getFinder, setFinder] = createContext<() => Finder>();

/**
 * Which dialog is open, kept in the address.
 *
 * Every dialog in the panel used to be a piece of component state, so a reload
 * put a reader back on the list behind it and a link to what they were looking at
 * did not exist. The address carries it now: the dialog's name, and whatever it
 * needs to find its subject again.
 *
 * It rides the query string rather than the path. A dialog is a thing on top of a
 * view, not a different view, and every view can raise one - a path segment would
 * have to be understood by every route the panel parses, and the panel's own
 * route writer would then have to know which of them are dialogs. The query says
 * the same thing without teaching the router a second grammar, and it survives a
 * reload and a paste into a colleague's window just as well.
 */

const DIALOG_KEY = 'dialog';

export interface OpenDialog {
  /** Matches the `id` the dialog is declared with, so the two cannot drift. */
  name: string;
  params: Readonly<Record<string, string>>;
}

export function parseDialog(search: string): OpenDialog | null {
  /* The reactive class is for a URLSearchParams something reads through. This one
     is read to the end and thrown away inside the call, and what components watch
     is the router's own state. */
  // eslint-disable-next-line svelte/prefer-svelte-reactivity
  const query = new URLSearchParams(search);
  const name = query.get(DIALOG_KEY)?.trim() ?? '';
  if (name === '') return null;

  const params: Record<string, string> = {};
  for (const [key, value] of query) {
    if (key !== DIALOG_KEY) params[key] = value;
  }

  return { name, params };
}

/** The search string for a dialog, `''` for none, so it can be assigned straight onto a URL. */
export function dialogSearch(dialog: OpenDialog | null): string {
  if (dialog === null) return '';
  // Built and serialised in the same expression - see parseDialog above.
  // eslint-disable-next-line svelte/prefer-svelte-reactivity
  const query = new URLSearchParams({ [DIALOG_KEY]: dialog.name, ...dialog.params });
  return `?${query.toString()}`;
}

interface HistoryLike {
  readonly state: unknown;
  pushState(data: unknown, unused: string, url: string): void;
  replaceState(data: unknown, unused: string, url: string): void;
  back(): void;
}

/**
 * Stamped on the entries this router adds.
 *
 * Closing has to undo its own entry rather than stack a new one, or Back after
 * dismissing a dialog re-opens it. Counting the pushes is not enough to know
 * which entry is ours: a reload starts the count at zero on an entry we did
 * push, and raising a second dialog on top of the first pushes once but is
 * closed once. The mark travels with the entry, so it survives both.
 */
const OWN_ENTRY = { smyklotDialog: true };

function isOwnEntry(state: unknown): boolean {
  return typeof state === 'object' && state !== null && 'smyklotDialog' in state;
}

interface BrowserLike {
  readonly location: { readonly pathname: string; readonly search: string };
  readonly history: HistoryLike;
  addEventListener(type: 'popstate', listener: () => void): void;
  removeEventListener(type: 'popstate', listener: () => void): void;
}

class DialogRouter {
  #current = $state<OpenDialog | null>(null);
  #browser: BrowserLike | null = null;

  /** What the address says is open. Read it in a component and it re-reads on navigation. */
  get current(): OpenDialog | null {
    return this.#current;
  }

  /** True when this dialog is the one the address names. */
  isOpen(name: string): boolean {
    return this.#current?.name === name;
  }

  /** A parameter of the open dialog, or `undefined` when a different one is open. */
  param(name: string, key: string): string | undefined {
    return this.#current?.name === name ? this.#current.params[key] : undefined;
  }

  attach(browser: BrowserLike): () => void {
    this.#browser = browser;
    this.#current = parseDialog(browser.location.search);
    const onPopState = (): void => {
      this.#current = parseDialog(browser.location.search);
    };
    browser.addEventListener('popstate', onPopState);

    return () => {
      browser.removeEventListener('popstate', onPopState);
      this.#browser = null;
    };
  }

  open(name: string, params: Readonly<Record<string, string>> = {}): void {
    const next: OpenDialog = { name, params };
    const replacing = this.#current !== null;
    this.#current = next;
    const browser = this.#browser;
    if (browser === null) return;

    /* One dialog raised from inside another swaps the entry rather than adding
       one. Two entries would take two presses of Back to leave what reads as one
       piece of work, and would leave the first dialog re-opening behind the
       second. */
    const url = browser.location.pathname + dialogSearch(next);
    if (replacing) {
      browser.history.replaceState(browser.history.state, '', url);
      return;
    }
    browser.history.pushState(OWN_ENTRY, '', url);
  }

  /** Changes a parameter of the dialog already open, without stacking another entry. */
  update(name: string, params: Readonly<Record<string, string>>): void {
    if (this.#current?.name !== name) return;
    const next: OpenDialog = { name, params: { ...this.#current.params, ...params } };
    this.#current = next;
    const browser = this.#browser;
    if (browser === null) return;
    browser.history.replaceState(
      browser.history.state,
      '',
      browser.location.pathname + dialogSearch(next),
    );
  }

  close(): void {
    if (this.#current === null) return;
    this.#current = null;
    const browser = this.#browser;
    if (browser === null) return;

    /* Undo the entry this router added, so Back after closing goes where the
       reader came from rather than re-opening what they just dismissed. Landing
       here from a pasted link or a reload means there is no entry of ours behind
       this one, and the query is dropped in place instead. */
    if (isOwnEntry(browser.history.state)) {
      browser.history.back();
      return;
    }
    browser.history.replaceState(browser.history.state, '', browser.location.pathname);
  }
}

export const dialogRoute = new DialogRouter();

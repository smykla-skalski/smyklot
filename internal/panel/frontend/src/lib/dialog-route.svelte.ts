/**
 * Which dialog is open, kept in the address.
 *
 * Every dialog in the panel used to be a piece of component state, so a reload
 * put a reader back on the list behind it and a link to what they were looking at
 * did not exist. The address carries it now.
 *
 * A dialog about a row of a view reads as part of that view's path:
 * `/i/acme/repositories/api-gateway/file`. The grammar for those lives in
 * `route-dialogs`, and this router writes them through the panel's own route
 * writer, so there is one place that decides what a panel address looks like.
 *
 * A dialog with no view to sit on - the notification inbox, which any view can
 * raise - has no such path, and rides the query string until it becomes a view of
 * its own. Both kinds are read and opened through the same three calls, so a
 * component never has to know which kind it is.
 */

import { panelRoutePath, parsePanelRoute, type PanelRoute, type RouteDialog } from './routes';
import { dialogSegments, isDialogHost } from './route-dialogs';

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

/** Whether this dialog has a place in its view's path, or has to ride the query. */
export function hasRouteHome(route: PanelRoute | null, dialog: OpenDialog): boolean {
  const view = hostView(route);

  return view !== null && dialogSegments(view, dialog) !== null;
}

function hostView(route: PanelRoute | null) {
  if (route === null) return null;
  const view = 'rootView' in route ? rootHost(route) : route.view;

  return view !== null && isDialogHost(view) ? view : null;
}

function rootHost(route: Extract<PanelRoute, { rootView: string }>): string | null {
  if (route.rootView === 'installation') return route.view;
  if (route.rootView === 'access-users' || route.rootView === 'access-invitations') {
    return route.rootView;
  }

  return null;
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
  #basePath = '';

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

  attach(browser: BrowserLike, basePath = ''): () => void {
    this.#browser = browser;
    this.#basePath = basePath;
    this.#current = this.#read();
    const onPopState = (): void => {
      this.#current = this.#read();
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
    const url = this.#url(next);
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
    browser.history.replaceState(browser.history.state, '', this.#url(next));
  }

  close(): void {
    if (this.#current === null) return;
    this.#current = null;
    const browser = this.#browser;
    if (browser === null) return;

    /* Undo the entry this router added, so Back after closing goes where the
       reader came from rather than re-opening what they just dismissed. Landing
       here from a pasted link or a reload means there is no entry of ours behind
       this one, and the address is rewritten in place instead. */
    if (isOwnEntry(browser.history.state)) {
      browser.history.back();
      return;
    }
    browser.history.replaceState(browser.history.state, '', this.#url(null));
  }

  /** The address for a dialog: in the view's path when it has a place there. */
  #url(dialog: OpenDialog | null): string {
    const browser = this.#browser;
    if (browser === null) return '';
    const route = parsePanelRoute(this.#basePath, browser.location.pathname);

    if (dialog !== null && hasRouteHome(route, dialog)) {
      return panelRoutePath(this.#basePath, withDialog(route, dialog));
    }
    /* Either there is no path for it, or it is being closed. Closing writes the
       bare view, which drops a path dialog and a query one alike. */
    const bare =
      route === null
        ? browser.location.pathname
        : panelRoutePath(this.#basePath, withDialog(route, null));

    return bare + dialogSearch(dialog === null || hasRouteHome(route, dialog) ? null : dialog);
  }

  #read(): OpenDialog | null {
    const browser = this.#browser;
    if (browser === null) return null;
    const route = parsePanelRoute(this.#basePath, browser.location.pathname);
    const fromPath = route === null ? undefined : routeDialog(route);
    if (fromPath !== undefined) return fromPath;

    return parseDialog(browser.location.search);
  }
}

function routeDialog(route: PanelRoute): RouteDialog | undefined {
  return 'dialog' in route ? route.dialog : undefined;
}

function withDialog(route: PanelRoute | null, dialog: OpenDialog | null): PanelRoute {
  if (route === null)
    throw new Error('cannot place a dialog on an address that is not a panel route');
  const next = { ...route } as PanelRoute & { dialog?: RouteDialog };
  if (dialog === null) {
    delete next.dialog;

    return next;
  }
  next.dialog = dialog;

  return next;
}

export const dialogRoute = new DialogRouter();

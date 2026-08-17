/**
 * Which dialog is open, kept in the address.
 *
 * Under SvelteKit, path-form dialogs live in the [...rest] route param and
 * query-form dialogs live in the URL query and page.state (shallow routing).
 * This adapter reads both and exposes the same interface the view components
 * already use.
 */

import { page } from '$app/state';
import { goto } from '$app/navigation';
import { SvelteURLSearchParams } from 'svelte/reactivity';

import {
  dialogSegments,
  isDialogHost,
  parseDialogSegments,
  type DialogHost,
  type RouteDialog,
} from './route-dialogs';
import { panelAddress } from './addresses';
import { basePath } from './paths';
import type { PanelRoute, RootRoute } from './routes';

export type { RouteDialog } from './route-dialogs';

const DIALOG_KEY = 'dialog';

export interface OpenDialog {
  name: string;
  params: Readonly<Record<string, string>>;
}

interface DialogPageState {
  dialog?: OpenDialog;
  smyklotDialogEntry?: true;
  smyklotDialogClosed?: true;
}

function dialogState(): DialogPageState {
  return page.state as DialogPageState;
}

function withoutDialogState(): App.PageState {
  const state = { ...dialogState() };
  delete state.dialog;
  delete state.smyklotDialogEntry;
  state.smyklotDialogClosed = true;
  return state;
}

function isRootInstallation(): boolean {
  return page.url.pathname.startsWith(`${basePath}/root/installations/`);
}

/**
 * The address as it stands, with a different query.
 *
 * Whole, base and all: `panelAddress` returns addresses SvelteKit has already resolved,
 * so the two are the same kind of thing and either can be navigated to directly. This
 * used to strip the base off so the caller could put it back on.
 */
function currentPanelAddress(search: string): string {
  return `${page.url.pathname}${search}${page.url.hash}`;
}

export function parseDialog(search: string): OpenDialog | null {
  // Read once and discard; the reactive source is page.url.
  const query = new SvelteURLSearchParams(search);
  const name = query.get(DIALOG_KEY)?.trim() ?? '';
  if (name === '') return null;

  const params: Record<string, string> = {};
  for (const [key, value] of query) {
    if (key !== DIALOG_KEY) params[key] = value;
  }

  return { name, params };
}

export function dialogSearch(dialog: OpenDialog | null): string {
  if (dialog === null) return '';
  // Built and serialized in one expression; see parseDialog above.
  const query = new SvelteURLSearchParams({ [DIALOG_KEY]: dialog.name, ...dialog.params });

  return `?${query.toString()}`;
}

export function legacyInboxRoute(search: string): boolean {
  return parseDialog(search)?.name === 'security-notifications';
}

function dialogHostFromPage(): DialogHost | null {
  const view = page.params.view;
  if (view !== undefined && isDialogHost(view)) return view;
  const section = page.params.section;
  if (section !== undefined) {
    const host = `access-${section}`;
    if (isDialogHost(host)) return host;
  }
  return null;
}

function pathForDialog(host: DialogHost, dialog: RouteDialog): string | null {
  if (dialogSegments(host, dialog) === null) return null;

  if (host === 'access-users' || host === 'access-invitations') {
    return panelAddress({ rootView: host, dialog } as RootRoute);
  }

  const view = page.params.view;
  const account = page.params.account;
  if (view === undefined || account === undefined || !isDialogHost(view)) return null;
  if (isRootInstallation()) {
    return panelAddress({ rootView: 'installation', account, view, dialog } as RootRoute);
  }
  return panelAddress({ account, view, dialog } as PanelRoute);
}

function bareHostPath(): string | null {
  const section = page.params.section;
  if (typeof section === 'string' && page.url.pathname.startsWith(`${basePath}/root/access/`)) {
    return panelAddress({
      rootView: section === 'users' ? 'access-users' : 'access-invitations',
    } as RootRoute);
  }

  const view = page.params.view;
  const account = page.params.account;
  if (view === undefined || account === undefined) return null;
  return isRootInstallation()
    ? panelAddress({ rootView: 'installation', account, view } as RootRoute)
    : panelAddress({ account, view } as PanelRoute);
}

class DialogRouter {
  get current(): OpenDialog | null {
    const state = dialogState();
    if (state.dialog !== undefined) return state.dialog;
    if (state.smyklotDialogClosed === true) return null;

    const host = dialogHostFromPage();
    const rest = page.params.rest;
    if (host !== null && typeof rest === 'string' && rest !== '') {
      const segments = rest.split('/').filter((s) => s !== '');
      if (segments.length > 0) {
        const dialog = parseDialogSegments(host, segments);
        if (dialog !== null) return dialog;
      }
    }

    return parseDialog(page.url.search);
  }

  isOpen(name: string): boolean {
    return this.current?.name === name;
  }

  param(name: string, key: string): string | undefined {
    const current = this.current;
    return current?.name === name ? current.params[key] : undefined;
  }

  /**
   * Writes a dialog into the address as a shallow entry.
   *
   * An entry this panel pushed itself is marked as owned, which `close` reads to
   * decide between stepping back through history and replacing the address. Pushing
   * always owns the entry it just made; replacing inherits whatever the entry it
   * lands on was.
   */
  private commit(dialog: OpenDialog, replace: boolean): void {
    const owned = !replace || dialogState().smyklotDialogEntry === true;
    const host = dialogHostFromPage();
    const path = host === null ? null : pathForDialog(host, dialog);
    const state: DialogPageState = {
      ...page.state,
      dialog,
      ...(owned ? { smyklotDialogEntry: true as const } : {}),
    };
    delete state.smyklotDialogClosed;
    goto(path ?? currentPanelAddress(dialogSearch(dialog)), { shallow: true, replace, state });
  }

  open(name: string, params: Readonly<Record<string, string>> = {}): void {
    // Replacing when a dialog is already open, so the pair does not leave two entries
    // for one overlay; pushing otherwise, so Back closes it.
    this.commit({ name, params }, this.current !== null);
  }

  update(name: string, params: Readonly<Record<string, string>>): void {
    if (this.current?.name !== name) return;

    this.commit({ name, params: { ...this.current.params, ...params } }, true);
  }

  close(): void {
    if (this.current === null) return;
    if (dialogState().smyklotDialogEntry === true) {
      history.back();
      return;
    }

    /**
     * The address the dialog hung off: the bare host path when the dialog was
     * segments of the path, and this address without its query when it was one.
     *
     * Shallow either way. The path form used to close with a `goto`, and that is
     * a navigation from the route that hosts a dialog to the route that does not
     * - two page components, so everything under the dialog was torn down and
     * built again. A reader who pressed Escape and started typing lost what they
     * typed, because the table they typed into was replaced by a new one seeded
     * from the last stored search, and their place in the list went with it.
     *
     * Nothing about the address decides which page component is mounted, and the
     * panel reads its route from the pathname rather than from `page.params`, so
     * the one already mounted renders the view underneath exactly as the other
     * would. A reload lands on the plain address and mounts that route properly.
     */
    const rest = page.params.rest;
    const path = bareHostPath();
    const target =
      rest !== undefined && rest !== '' && path !== null ? path : currentPanelAddress('');
    goto(target, { shallow: true, replace: true, state: withoutDialogState() });
  }
}

export const dialogRoute = new DialogRouter();

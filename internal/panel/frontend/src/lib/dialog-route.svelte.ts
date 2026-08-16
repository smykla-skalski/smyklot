/**
 * Which dialog is open, kept in the address.
 *
 * Under SvelteKit, path-form dialogs live in the [...rest] route param and
 * query-form dialogs live in the URL query and page.state (shallow routing).
 * This adapter reads both and exposes the same interface the view components
 * already use.
 */

import { page } from '$app/state';
import { goto, pushState, replaceState } from '$app/navigation';
import { base, resolve } from '$app/paths';
import type { Pathname } from '$app/types';
import { SvelteURLSearchParams } from 'svelte/reactivity';

import {
  dialogSegments,
  isDialogHost,
  parseDialogSegments,
  type DialogHost,
  type RouteDialog,
} from './route-dialogs';
import { panelRoutePath, type PanelRoute, type RootRoute } from './routes';

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
  return page.url.pathname.startsWith(`${base}/root/installations/`);
}

function currentPanelPath(search = page.url.search): string {
  const pathname =
    base !== '' && page.url.pathname.startsWith(base)
      ? page.url.pathname.slice(base.length)
      : page.url.pathname;
  return `${pathname}${search}${page.url.hash}`;
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
  if (view !== undefined && isDialogHost(view)) return view as DialogHost;
  const section = page.params.section;
  if (section !== undefined) {
    const host = `access-${section}`;
    if (isDialogHost(host)) return host as DialogHost;
  }
  return null;
}

function pathForDialog(host: DialogHost, dialog: RouteDialog): string | null {
  const segments = dialogSegments(host, dialog);
  if (segments === null) return null;

  const suffix = segments.map((segment) => `/${encodeURIComponent(segment)}`).join('');
  const section = page.params.section;
  if (typeof section === 'string' && (host === 'access-users' || host === 'access-invitations')) {
    return `/root/access/${section}${suffix}`;
  }

  const view = page.params.view;
  const account = page.params.account;
  if (view === undefined || account === undefined || !isDialogHost(view)) return null;
  if (isRootInstallation()) {
    return panelRoutePath('', { rootView: 'installation', account, view, dialog } as RootRoute);
  }
  return panelRoutePath('', { account, view, dialog } as PanelRoute);
}

function bareHostPath(): string | null {
  const section = page.params.section;
  if (typeof section === 'string' && page.url.pathname.startsWith(`${base}/root/access/`)) {
    return `/root/access/${section}`;
  }

  const view = page.params.view;
  const account = page.params.account;
  if (view === undefined || account === undefined) return null;
  return isRootInstallation()
    ? panelRoutePath('', { rootView: 'installation', account, view } as RootRoute)
    : panelRoutePath('', { account, view } as PanelRoute);
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

  open(name: string, params: Readonly<Record<string, string>> = {}): void {
    const replacing = this.current !== null;
    const host = dialogHostFromPage();
    const dialog: RouteDialog = { name, params };
    const path = host === null ? null : pathForDialog(host, dialog);
    const state: DialogPageState = {
      ...page.state,
      dialog: { name, params },
      ...(!replacing || dialogState().smyklotDialogEntry === true
        ? { smyklotDialogEntry: true as const }
        : {}),
    };
    delete state.smyklotDialogClosed;
    const navigate = replacing ? replaceState : pushState;
    navigate(resolve((path ?? currentPanelPath(dialogSearch(dialog))) as Pathname), state);
  }

  update(name: string, params: Readonly<Record<string, string>>): void {
    if (this.current?.name !== name) return;
    const next: OpenDialog = { name, params: { ...this.current.params, ...params } };
    const host = dialogHostFromPage();
    const path = host === null ? null : pathForDialog(host, next);
    const state: DialogPageState = {
      ...page.state,
      dialog: next,
      ...(dialogState().smyklotDialogEntry === true ? { smyklotDialogEntry: true } : {}),
    };
    delete state.smyklotDialogClosed;
    replaceState(resolve((path ?? currentPanelPath(dialogSearch(next))) as Pathname), state);
  }

  close(): void {
    if (this.current === null) return;
    if (dialogState().smyklotDialogEntry === true) {
      history.back();
      return;
    }

    const rest = page.params.rest;
    const path = bareHostPath();
    if (typeof rest === 'string' && rest !== '' && path !== null) {
      goto(resolve(path as Pathname), { replaceState: true, state: withoutDialogState() });
    } else {
      replaceState(resolve(currentPanelPath('') as Pathname), withoutDialogState());
    }
  }
}

export const dialogRoute = new DialogRouter();

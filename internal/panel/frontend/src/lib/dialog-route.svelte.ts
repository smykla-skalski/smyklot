/**
 * Which dialog is open, kept in the address.
 *
 * Under SvelteKit, path-form dialogs live in the [...rest] route param and
 * query-form dialogs live in page.state (shallow routing). This adapter reads
 * both and exposes the same interface the view components already use.
 */

import { page } from '$app/state';
import { goto, pushState, replaceState } from '$app/navigation';
import { base } from '$app/paths';

import {
  dialogSegments,
  isDialogHost,
  parseDialogSegments,
  type DialogHost,
  type RouteDialog,
} from './route-dialogs';
import { panelRoutePath, type PanelRoute, type PersonalRoute, type RootRoute } from './routes';

export type { RouteDialog } from './route-dialogs';

/**
 * The inbox as a dialog, which is what it was before it became a page.
 *
 * `?dialog=security-notifications` was its address for a few releases. A bookmark
 * from then opens the inbox rather than whichever view it happened to be standing
 * on, and the address is rewritten before anything reads it, so the name of a
 * dialog nothing will open never enters the router.
 */
export function legacyInboxRoute(search: string): PersonalRoute | null {
  return parseDialog(search)?.name === 'security-notifications' ? { personal: 'inbox' } : null;
}

export interface OpenDialog {
  name: string;
  params: Readonly<Record<string, string>>;
}

export function parseDialog(search: string): OpenDialog | null {
  // eslint-disable-next-line svelte/prefer-svelte-reactivity
  const query = new URLSearchParams(search);
  const name = query.get('dialog')?.trim() ?? '';
  if (name === '') return null;
  const params: Record<string, string> = {};
  for (const [key, value] of query) {
    if (key !== 'dialog') params[key] = value;
  }
  return { name, params };
}

export function dialogSearch(dialog: OpenDialog | null): string {
  if (dialog === null) return '';
  // eslint-disable-next-line svelte/prefer-svelte-reactivity
  const query = new URLSearchParams({ dialog: dialog.name, ...dialog.params });
  return `?${query.toString()}`;
}

export function hasRouteHome(route: PanelRoute | null, dialog: OpenDialog): boolean {
  if (route === null) return false;
  const view =
    'rootView' in route && route.rootView === 'installation'
      ? route.view
      : 'rootView' in route &&
          (route.rootView === 'access-users' || route.rootView === 'access-invitations')
        ? route.rootView
        : 'view' in route
          ? route.view
          : null;
  return view !== null && isDialogHost(view) && dialogSegments(view as DialogHost, dialog) !== null;
}

function isRootInstallation(): boolean {
  return page.url.pathname.startsWith(`${base}/root/installations/`);
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

class DialogRouter {
  get current(): OpenDialog | null {
    const host = dialogHostFromPage();
    const rest = page.params.rest;
    if (host !== null && typeof rest === 'string' && rest !== '') {
      const segments = rest.split('/').filter((s) => s !== '');
      if (segments.length > 0) {
        const dialog = parseDialogSegments(host, segments);
        if (dialog !== null) return dialog;
      }
    }
    const state = page.state as { dialog?: OpenDialog };
    return state.dialog ?? null;
  }

  isOpen(name: string): boolean {
    return this.current?.name === name;
  }

  param(name: string, key: string): string | undefined {
    const current = this.current;
    return current?.name === name ? current.params[key] : undefined;
  }

  open(name: string, params: Readonly<Record<string, string>> = {}): void {
    const view = page.params.view;
    const account = page.params.account;
    if (view !== undefined && account !== undefined && isDialogHost(view)) {
      const dialog: RouteDialog = { name, params };
      const segments = dialogSegments(view as DialogHost, dialog);
      if (segments !== null) {
        if (isRootInstallation()) {
          goto(
            panelRoutePath('', { rootView: 'installation', account, view, dialog } as RootRoute),
          );
        } else {
          goto(panelRoutePath('', { account, view, dialog } as PanelRoute));
        }
        return;
      }
    }
    pushState(page.url, { dialog: { name, params } });
  }

  update(name: string, params: Readonly<Record<string, string>>): void {
    if (this.current?.name !== name) return;
    const next: OpenDialog = { name, params: { ...this.current.params, ...params } };
    const view = page.params.view;
    const account = page.params.account;
    if (view !== undefined && account !== undefined && isDialogHost(view)) {
      const dialog: RouteDialog = next;
      const segments = dialogSegments(view as DialogHost, dialog);
      if (segments !== null) {
        if (isRootInstallation()) {
          goto(
            panelRoutePath('', { rootView: 'installation', account, view, dialog } as RootRoute),
            {
              replaceState: true,
            },
          );
        } else {
          goto(panelRoutePath('', { account, view, dialog } as PanelRoute), { replaceState: true });
        }
        return;
      }
    }
    replaceState(page.url, { dialog: next });
  }

  close(): void {
    if (this.current === null) return;
    const view = page.params.view;
    const account = page.params.account;
    const rest = page.params.rest;
    if (view !== undefined && account !== undefined && typeof rest === 'string' && rest !== '') {
      if (isRootInstallation()) {
        goto(panelRoutePath('', { rootView: 'installation', account, view } as RootRoute), {
          replaceState: true,
        });
      } else {
        goto(panelRoutePath('', { account, view } as PanelRoute), { replaceState: true });
      }
    } else {
      replaceState(page.url, {});
    }
  }

  attach(): () => void {
    return () => {};
  }
}

export const dialogRoute = new DialogRouter();

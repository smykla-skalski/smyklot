import { normalizeBasePath } from './base.ts';
import {
  dialogSegments,
  isDialogHost,
  parseDialogSegments,
  type RouteDialog,
} from './route-dialogs.ts';

export type { RouteDialog };

export const PANEL_VIEWS = ['settings', 'repositories', 'users', 'invitations', 'history'] as const;
const SCOPED_PANEL_VIEWS = ['settings', 'repositories', 'users', 'invitations', 'history'] as const;

/**
 * The views that belong to the reader rather than to a workspace or the console.
 *
 * Everything else the panel shows is scoped to something in the address: an
 * installation under `/i/<account>`, the application under `/root`. The inbox is
 * scoped to whoever is signed in, which no path segment can name, so it sits at
 * the top: `/inbox`. One address, and the same page whichever part of the panel
 * it was reached from - a Root who opens it leaves the console rather than
 * carrying it along, because an address that says nothing about the console
 * cannot be reloaded back into one.
 */
export const PERSONAL_VIEWS = ['inbox'] as const;

export const HISTORY_SECTIONS = ['audit', 'failures'] as const;

export type PanelView = (typeof PANEL_VIEWS)[number];
export type ScopedPanelView = (typeof SCOPED_PANEL_VIEWS)[number];
export type PersonalView = (typeof PERSONAL_VIEWS)[number];
/** History's two tables are addressable, so a reload lands where you left off. */
export type HistorySection = (typeof HISTORY_SECTIONS)[number];
export type RootSection =
  'overview' | 'queue' | 'installations' | 'access' | 'history' | 'settings';
export type PanelSection = Exclude<ScopedPanelView, 'users' | 'invitations'> | 'access';
export type RootRoute =
  | {
      rootView: 'overview' | 'installations' | 'access-users' | 'access-invitations';
      dialog?: RouteDialog;
    }
  | { rootView: 'history-audit' | 'history-failures' | 'settings' }
  /**
   * Work the service has accepted and will do later, on a schedule it chooses.
   *
   * One word, like every other item in this nav, and a generic one: what is in it
   * today is pull requests waiting on their checks, and the data model already
   * carries cleanup retries. "Merge queue" is GitHub's name for batching pull
   * requests and testing them together, which is not this.
   */
  | { rootView: 'queue' | 'queue-recent' }
  /** A request is a page rather than a dialog: the timeline is a record to link someone to. */
  | { rootView: 'queue-request'; request: string }
  | {
      rootView: 'installation';
      account: string;
      view: ScopedPanelView;
      section?: HistorySection;
      dialog?: RouteDialog;
    };

export type InstallationRoute = {
  account: string;
  view: ScopedPanelView;
  section?: HistorySection;
  /** What is open on top of the view; see `route-dialogs`. */
  dialog?: RouteDialog;
};
/** A page of the reader's own, standing beside the workspaces and the console. */
export type PersonalRoute = { personal: PersonalView };
export type PanelRoute = InstallationRoute | RootRoute | PersonalRoute;

export interface ResolvedPanelRoute {
  account: string;
  view: PanelView;
  section?: HistorySection;
}

export function parsePanelRoute(basePath: string, pathname: string): PanelRoute | null {
  const base = normalizeBasePath(basePath);
  if (base !== '' && pathname !== base && !pathname.startsWith(`${base}/`)) return null;

  const relative = pathname.slice(base.length).replace(/^\/+|\/+$/g, '');
  if (relative === '') return null;

  const parts = relative.split('/');
  if (parts[0] === 'root') return parseRootRoute(parts);
  /* A personal view is the whole address. Nothing hangs off it - no account to
     scope it to and no dialog to stand on it - so anything further is a path
     that does not resolve rather than the view with something appended. */
  const personal = PERSONAL_VIEWS.find((view) => view === parts[0]);
  if (personal !== undefined) return parts.length === 1 ? { personal } : null;
  if (parts.length < 3) return null;

  const [namespace, encodedAccount, rawView] = parts;
  if (
    namespace !== 'i' ||
    encodedAccount === undefined ||
    encodedAccount.length === 0 ||
    rawView === undefined ||
    !isScopedPanelView(rawView)
  )
    return null;

  /* Everything past the view is either history's table or a dialog. A view that
     hosts dialogs never has a section, and one that has a section hosts none, so
     the two grammars cannot be confused for each other. */
  const trailing = parts.slice(3);
  const dialog = parseTrailingDialog(rawView, trailing);
  if (dialog === 'invalid') return null;

  const section = parseSection(rawView, dialog === undefined ? trailing[0] : undefined);
  if (section === 'invalid' || (dialog === undefined && trailing.length > 1)) return null;

  let account: string;
  try {
    account = decodeURIComponent(encodedAccount);
  } catch {
    return null;
  }

  if (account.trim() === '') return null;
  const route: InstallationRoute = { account, view: rawView };
  if (dialog !== undefined) return { ...route, dialog };
  return section === undefined ? route : { ...route, section };
}

/**
 * Reads the segments past a view as a dialog.
 *
 * `undefined` for a view that hosts none or a path that carries none;
 * `'invalid'` for segments that were meant to be one and are not, which is an
 * address that does not resolve rather than the bare view - a mistyped
 * repository name should say so rather than quietly showing the list.
 */
function parseTrailingDialog(
  view: string,
  segments: string[],
): RouteDialog | undefined | 'invalid' {
  if (segments.length === 0 || !isDialogHost(view)) return undefined;

  return parseDialogSegments(view, segments) ?? 'invalid';
}

export function parseInvitationToken(basePath: string, pathname: string): string | null {
  const base = normalizeBasePath(basePath);
  if (base !== '' && !pathname.startsWith(`${base}/`)) return null;
  const relative = pathname.slice(base.length).replace(/^\/+|\/+$/g, '');
  const parts = relative.split('/');
  if (parts.length !== 2 || parts[0] !== 'invite' || parts[1] === undefined) return null;
  try {
    const token = decodeURIComponent(parts[1]);
    return token.trim() === '' ? null : token;
  } catch {
    return null;
  }
}

export function panelRoutePath(basePath: string, route: PanelRoute): string {
  const base = normalizeBasePath(basePath);
  if ('rootView' in route) return `${base}${rootRoutePath(route)}`;
  if ('personal' in route) return `${base}/${route.personal}`;

  return (
    `${base}/i/${encodeURIComponent(route.account)}/${route.view}` +
    sectionSuffix(route) +
    dialogSuffix(route.view, route.dialog)
  );
}

/** The path segments an open dialog adds, already escaped. */
function dialogSuffix(view: string, dialog: RouteDialog | undefined): string {
  if (dialog === undefined || !isDialogHost(view)) return '';
  const segments = dialogSegments(view, dialog);
  if (segments === null) return '';

  return segments.map((segment) => `/${encodeURIComponent(segment)}`).join('');
}

export function panelDocumentTitle(route: PanelRoute): string {
  const rootConsole = 'rootView' in route;
  const segments = routeTitleSegments(route);
  if (rootConsole) segments.push('root-console');
  return [...segments.map(routeSegmentLabel), 'SMYKLOT'].join(' | ');
}

export function panelViewSection(view: ScopedPanelView): PanelSection {
  return view === 'users' || view === 'invitations' ? 'access' : view;
}

export function routeSegmentLabel(segment: string): string {
  return segment
    .split('-')
    .map((word) => word.slice(0, 1).toUpperCase() + word.slice(1))
    .join(' ');
}

export function resolvePanelRoute(
  availableAccounts: readonly string[],
  requested: InstallationRoute | null,
  preferredAccount: string | null,
): ResolvedPanelRoute | null {
  const requestedAccount = findAccount(availableAccounts, requested?.account ?? null);
  const account =
    requestedAccount ?? findAccount(availableAccounts, preferredAccount) ?? availableAccounts[0];
  if (account === undefined) return null;

  const view = requested?.view ?? 'settings';
  /* History always resolves to a named table, so the address bar never sits on
     a bare /history that a reload would have to guess at. */
  return view === 'history'
    ? { account, view, section: requested?.section ?? 'audit' }
    : { account, view };
}

export function rootSection(route: RootRoute): RootSection {
  if (route.rootView === 'access-users' || route.rootView === 'access-invitations') return 'access';
  if (route.rootView === 'history-audit' || route.rootView === 'history-failures') return 'history';
  if (route.rootView === 'installation') return 'installations';
  if (route.rootView === 'queue-recent' || route.rootView === 'queue-request') return 'queue';
  return route.rootView;
}

export function rootSectionRoute(section: RootSection): RootRoute {
  if (section === 'access') return { rootView: 'access-users' };
  if (section === 'history') return { rootView: 'history-audit' };
  return { rootView: section };
}

function isScopedPanelView(value: string): value is ScopedPanelView {
  return SCOPED_PANEL_VIEWS.some((view) => view === value);
}

function routeTitleSegments(route: PanelRoute): string[] {
  if ('personal' in route) return [route.personal];
  if ('rootView' in route && route.rootView !== 'installation') {
    return route.rootView.split('-').reverse();
  }
  const view = route.view;
  const section = panelViewSection(view);
  const leaf = route.section ?? view;
  return leaf === section ? [leaf] : [leaf, section];
}

/** `undefined` for "no segment", `'invalid'` for a segment that cannot be one. */
function parseSection(
  view: string,
  raw: string | undefined,
): HistorySection | undefined | 'invalid' {
  if (raw === undefined) return undefined;
  if (view !== 'history') return 'invalid';
  return HISTORY_SECTIONS.find((section) => section === raw) ?? 'invalid';
}

function sectionSuffix(route: { view: ScopedPanelView; section?: HistorySection }): string {
  return route.view === 'history' && route.section !== undefined ? `/${route.section}` : '';
}

function parseRootRoute(parts: string[]): RootRoute | null {
  if (parts.length === 1) return { rootView: 'overview' };
  if (parts.length === 2 && parts[1] === 'installations') return { rootView: 'installations' };
  if (parts.length === 2 && parts[1] === 'settings') return { rootView: 'settings' };
  if (parts.length >= 3 && parts[1] === 'access') {
    /* The Root console's tables take the same dialog grammar as an
       installation's, because they list the same things. */
    const host = parts[2] === 'users' ? 'access-users' : 'access-invitations';
    if (parts[2] === 'users' || parts[2] === 'invitations') {
      if (parts.length === 3) return { rootView: host };
      const dialog = parseDialogSegments(host, parts.slice(3));

      return dialog === null ? null : { rootView: host, dialog };
    }
  }
  if (parts.length === 2 && parts[1] === 'queue') return { rootView: 'queue' };
  if (parts.length === 3 && parts[1] === 'queue' && parts[2] === 'recent') {
    return { rootView: 'queue-recent' };
  }
  if (parts.length === 4 && parts[1] === 'queue' && parts[2] === 'request') {
    let request: string;
    try {
      request = decodeURIComponent(parts[3] ?? '');
    } catch {
      return null;
    }

    return request.trim() === '' ? null : { rootView: 'queue-request', request };
  }
  if (parts.length === 3 && parts[1] === 'history') {
    if (parts[2] === 'audit') return { rootView: 'history-audit' };
    if (parts[2] === 'failures') return { rootView: 'history-failures' };
  }
  /* A bare section path is a legitimate address - somebody typed or bookmarked
     it - and resolves to that section's default table rather than falling
     through to the installation default. */
  if (parts.length === 2 && parts[1] === 'history') return { rootView: 'history-audit' };
  if (parts.length === 2 && parts[1] === 'access') return { rootView: 'access-users' };
  if (parts.length < 4 || parts[1] !== 'installations' || !isScopedPanelView(parts[3] ?? '')) {
    return null;
  }

  const view = parts[3] as ScopedPanelView;
  const trailing = parts.slice(4);
  const dialog = parseTrailingDialog(view, trailing);
  if (dialog === 'invalid') return null;

  const section = parseSection(view, dialog === undefined ? trailing[0] : undefined);
  if (section === 'invalid' || (dialog === undefined && trailing.length > 1)) return null;

  let account: string;
  try {
    account = decodeURIComponent(parts[2] ?? '');
  } catch {
    return null;
  }
  if (account.trim() === '') return null;
  const route: RootRoute = { rootView: 'installation', account, view };
  if (dialog !== undefined) return { ...route, dialog };
  return section === undefined ? route : { ...route, section };
}

function rootRoutePath(route: RootRoute): string {
  if (route.rootView === 'installation')
    return (
      `/root/installations/${encodeURIComponent(route.account)}/${route.view}` +
      sectionSuffix(route) +
      dialogSuffix(route.view, route.dialog)
    );
  if (route.rootView === 'overview') return '/root';
  if (route.rootView === 'installations') return '/root/installations';
  if (route.rootView === 'access-users')
    return `/root/access/users${dialogSuffix('access-users', route.dialog)}`;
  if (route.rootView === 'access-invitations')
    return `/root/access/invitations${dialogSuffix('access-invitations', route.dialog)}`;
  if (route.rootView === 'history-audit') return '/root/history/audit';
  if (route.rootView === 'history-failures') return '/root/history/failures';
  if (route.rootView === 'queue') return '/root/queue';
  if (route.rootView === 'queue-recent') return '/root/queue/recent';
  if (route.rootView === 'queue-request')
    return `/root/queue/request/${encodeURIComponent(route.request)}`;
  return '/root/settings';
}

function findAccount(accounts: readonly string[], requested: string | null): string | undefined {
  if (requested === null) return undefined;
  const folded = requested.toLowerCase();
  return accounts.find((account) => account.toLowerCase() === folded);
}

import { normalizeBasePath } from './base';

export const PANEL_VIEWS = ['settings', 'repositories', 'users', 'invitations', 'history'] as const;
const SCOPED_PANEL_VIEWS = ['settings', 'repositories', 'users', 'invitations', 'history'] as const;

export const HISTORY_SECTIONS = ['audit', 'failures'] as const;

export type PanelView = (typeof PANEL_VIEWS)[number];
export type ScopedPanelView = (typeof SCOPED_PANEL_VIEWS)[number];
/** History's two tables are addressable, so a reload lands where you left off. */
export type HistorySection = (typeof HISTORY_SECTIONS)[number];
export type RootSection = 'overview' | 'installations' | 'access' | 'history' | 'settings';
export type PanelSection = Exclude<ScopedPanelView, 'users' | 'invitations'> | 'access';
export type RootRoute =
  | { rootView: 'overview' | 'installations' | 'access-users' | 'access-invitations' }
  | { rootView: 'history-audit' | 'history-failures' | 'settings' }
  | {
      rootView: 'installation';
      account: string;
      view: ScopedPanelView;
      section?: HistorySection;
    };

export type InstallationRoute = {
  account: string;
  view: ScopedPanelView;
  section?: HistorySection;
};
export type PanelRoute = InstallationRoute | RootRoute;

export interface ResolvedPanelRoute {
  account: string;
  view: PanelView;
  section?: HistorySection;
}

interface BrowserNavigation {
  readonly location: Pick<Location, 'pathname'>;
  readonly history: Pick<History, 'pushState' | 'replaceState'>;
  addEventListener(type: 'popstate', listener: () => void): void;
  removeEventListener(type: 'popstate', listener: () => void): void;
}

export interface PanelRouter {
  current(): PanelRoute | null;
  path(route: PanelRoute): string;
  push(route: PanelRoute): void;
  replace(route: PanelRoute): void;
  subscribe(listener: (route: PanelRoute | null) => void): () => void;
}

export function parsePanelRoute(basePath: string, pathname: string): PanelRoute | null {
  const base = normalizeBasePath(basePath);
  if (base !== '' && pathname !== base && !pathname.startsWith(`${base}/`)) return null;

  const relative = pathname.slice(base.length).replace(/^\/+|\/+$/g, '');
  if (relative === '') return null;

  const parts = relative.split('/');
  if (parts[0] === 'root') return parseRootRoute(parts);
  if (parts.length !== 3 && parts.length !== 4) return null;

  const [namespace, encodedAccount, rawView] = parts;
  if (
    namespace !== 'i' ||
    encodedAccount === undefined ||
    encodedAccount.length === 0 ||
    rawView === undefined ||
    !isScopedPanelView(rawView)
  )
    return null;

  const section = parseSection(rawView, parts[3]);
  if (section === 'invalid') return null;

  let account: string;
  try {
    account = decodeURIComponent(encodedAccount);
  } catch {
    return null;
  }

  if (account.trim() === '') return null;
  const route: InstallationRoute = { account, view: rawView };
  return section === undefined ? route : { ...route, section };
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
  return `${base}/i/${encodeURIComponent(route.account)}/${route.view}${sectionSuffix(route)}`;
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

export function resolveDocumentTitleRoute(
  active: PanelRoute,
  requested: PanelRoute | null,
  routePending: boolean,
): PanelRoute {
  return routePending && requested !== null ? requested : active;
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
  return route.rootView;
}

export function rootSectionRoute(section: RootSection): RootRoute {
  if (section === 'access') return { rootView: 'access-users' };
  if (section === 'history') return { rootView: 'history-audit' };
  return { rootView: section };
}

export function createPanelRouter(basePath: string, browser: BrowserNavigation): PanelRouter {
  function current(): PanelRoute | null {
    return parsePanelRoute(basePath, browser.location.pathname);
  }

  function write(route: PanelRoute, replace: boolean): void {
    const next = panelRoutePath(basePath, route);
    if (next === browser.location.pathname) return;

    const method = replace ? browser.history.replaceState : browser.history.pushState;
    method.call(browser.history, null, '', next);
  }

  return {
    current,
    path: (route) => panelRoutePath(basePath, route),
    push: (route) => write(route, false),
    replace: (route) => write(route, true),
    subscribe(listener) {
      const handlePopState = (): void => listener(current());
      browser.addEventListener('popstate', handlePopState);
      return () => browser.removeEventListener('popstate', handlePopState);
    },
  };
}

function isScopedPanelView(value: string): value is ScopedPanelView {
  return SCOPED_PANEL_VIEWS.some((view) => view === value);
}

function routeTitleSegments(route: PanelRoute): string[] {
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
  if (parts.length === 3 && parts[1] === 'access') {
    if (parts[2] === 'users') return { rootView: 'access-users' };
    if (parts[2] === 'invitations') return { rootView: 'access-invitations' };
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
  if (
    (parts.length !== 4 && parts.length !== 5) ||
    parts[1] !== 'installations' ||
    !isScopedPanelView(parts[3] ?? '')
  ) {
    return null;
  }

  const view = parts[3] as ScopedPanelView;
  const section = parseSection(view, parts[4]);
  if (section === 'invalid') return null;

  let account: string;
  try {
    account = decodeURIComponent(parts[2] ?? '');
  } catch {
    return null;
  }
  if (account.trim() === '') return null;
  const route: RootRoute = { rootView: 'installation', account, view };
  return section === undefined ? route : { ...route, section };
}

function rootRoutePath(route: RootRoute): string {
  if (route.rootView === 'installation')
    return `/root/installations/${encodeURIComponent(route.account)}/${route.view}${sectionSuffix(route)}`;
  if (route.rootView === 'overview') return '/root';
  if (route.rootView === 'installations') return '/root/installations';
  if (route.rootView === 'access-users') return '/root/access/users';
  if (route.rootView === 'access-invitations') return '/root/access/invitations';
  if (route.rootView === 'history-audit') return '/root/history/audit';
  if (route.rootView === 'history-failures') return '/root/history/failures';
  return '/root/settings';
}

function findAccount(accounts: readonly string[], requested: string | null): string | undefined {
  if (requested === null) return undefined;
  const folded = requested.toLowerCase();
  return accounts.find((account) => account.toLowerCase() === folded);
}

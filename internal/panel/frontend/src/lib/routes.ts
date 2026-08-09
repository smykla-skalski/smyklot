import { normalizeBasePath } from './base';

export const PANEL_VIEWS = ['settings', 'repositories', 'history', 'help'] as const;
const SCOPED_PANEL_VIEWS = ['settings', 'repositories', 'history'] as const;

export type PanelView = (typeof PANEL_VIEWS)[number];
export type ScopedPanelView = (typeof SCOPED_PANEL_VIEWS)[number];

export type PanelRoute = { account: string; view: ScopedPanelView } | { view: 'help' };

export interface ResolvedPanelRoute {
  account: string;
  view: PanelView;
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
  if (relative === 'help') return { view: 'help' };

  const parts = relative.split('/');
  if (parts.length !== 3) return null;

  const [namespace, encodedAccount, rawView] = parts;
  if (
    namespace !== 'i' ||
    encodedAccount === undefined ||
    encodedAccount.length === 0 ||
    rawView === undefined ||
    !isScopedPanelView(rawView)
  )
    return null;

  let account: string;
  try {
    account = decodeURIComponent(encodedAccount);
  } catch {
    return null;
  }

  return account.trim() === '' ? null : { account, view: rawView };
}

export function panelRoutePath(basePath: string, route: PanelRoute): string {
  const base = normalizeBasePath(basePath);
  if (route.view === 'help') return `${base}/help`;
  return `${base}/i/${encodeURIComponent(route.account)}/${route.view}`;
}

export function resolvePanelRoute(
  availableAccounts: readonly string[],
  requested: PanelRoute | null,
  preferredAccount: string | null,
): ResolvedPanelRoute | null {
  const requestedAccount = findAccount(
    availableAccounts,
    requested?.view === 'help' ? null : (requested?.account ?? null),
  );
  const account =
    requestedAccount ?? findAccount(availableAccounts, preferredAccount) ?? availableAccounts[0];
  if (account === undefined) return null;

  return { account, view: requested?.view ?? 'settings' };
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

function findAccount(accounts: readonly string[], requested: string | null): string | undefined {
  if (requested === null) return undefined;
  const folded = requested.toLowerCase();
  return accounts.find((account) => account.toLowerCase() === folded);
}

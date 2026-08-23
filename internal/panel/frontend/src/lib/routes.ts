import { normalizeBasePath } from './base.ts';
import {
  decodeSegments,
  isDialogHost,
  parseDialogSegments,
  type DialogHost,
  type RouteDialog,
} from './route-dialogs.ts';

export type { RouteDialog };

export const PANEL_VIEWS = [
  'defaults',
  'repositories',
  'sync',
  'users',
  'invitations',
  'history',
] as const;

/** Views written directly after an installation account in the route tree. */
export const DIRECT_PANEL_VIEWS = ['defaults', 'repositories', 'sync', 'history'] as const;

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

/**
 * The views the Root console renders for one installation, which is not every
 * view an installation has.
 *
 * Sync is the difference. Configuring what an organization's repositories
 * should carry is the installation's own business, reached by elevating into
 * it, and the console reads through its own Root-scoped endpoints rather than
 * the ones an installation's members use. Accepting the address without
 * rendering anything is worse than refusing it: a bookmark that answers "this
 * view is unavailable" looks like a fault rather than a boundary.
 */
export const ROOT_INSTALLATION_VIEWS = [
  'defaults',
  'repositories',
  'users',
  'invitations',
  'history',
] as const;

/** Root installation views written directly after the installation account. */
export const DIRECT_ROOT_INSTALLATION_VIEWS = ['defaults', 'repositories', 'history'] as const;

export const HISTORY_SECTIONS = ['audit', 'failures'] as const;

/** The tables the Root console's access page is split into. */
export const ACCESS_SECTIONS = ['users', 'invitations'] as const;

/** The three addressable pages nested under Root Runtime. */
export const ROOT_RUNTIME_SECTIONS = ['service', 'database', 'settings'] as const;

/**
 * The panes of one repository's own page.
 *
 * A repository is reached at `/i/acme/repositories/api-gateway`, and the pane it
 * opens on rides the address the same way history's table does, so a link points
 * at the commands a colleague was asked to look at rather than at the file pane
 * everyone starts on.
 *
 * `file` is not written into the path. It is where the page opens, so the bare
 * repository already means it, and an address that says so twice is one a reader
 * would have to be told to ignore.
 */
export const REPOSITORY_SECTIONS = ['file', 'behavior', 'commands', 'sync'] as const;
export type RepositorySection = (typeof REPOSITORY_SECTIONS)[number];

/**
 * The sections of the sync view, each a sidebar row and an address.
 *
 * `overview` is not written into the path: it is where the view opens, so the
 * bare `/i/acme/sync` already means it - the same rule the repository page
 * applies to its `file` pane.
 */
export const WRITTEN_SYNC_SECTIONS = ['labels', 'settings', 'rulesets', 'files', 'plan'] as const;
export const SYNC_SECTIONS = ['overview', ...WRITTEN_SYNC_SECTIONS] as const;
export type SyncSection = (typeof SYNC_SECTIONS)[number];

/**
 * Which of the panes a surface can offer.
 *
 * Root manages somebody else's installation and sync has no Root address, so
 * whether there is anywhere to ask is what says the pane can be opened. Asked
 * once, here, rather than paired with `sync` at each of the places that would
 * otherwise have to remember: the switch's options and the fallback an address
 * naming a pane this surface has no answer for lands on.
 *
 * Takes what it needs as an argument and reaches for nothing. This module is
 * imported by `src/params.ts`, which the route-manifest build runs under plain
 * Node to write `routes.json` for the Go server - so a later pane answering
 * this from a store or a session would break that build with an error nowhere
 * near this file.
 */
export function availableRepositorySections(syncOffered: boolean): readonly RepositorySection[] {
  return REPOSITORY_SECTIONS.filter((section) => section !== 'sync' || syncOffered);
}

/** One repository, opened on one of its panes. */
export interface RepositoryPage {
  /** Named the way a person names it - `api-gateway`, never an id. */
  name: string;
  section: RepositorySection;
}

export type PanelView = (typeof PANEL_VIEWS)[number];

/**
 * A view in an installation's own address, which is every view there is.
 *
 * The name is kept because it says which surface an address belongs to - the
 * console's subset is `RootInstallationView` - but it is the same list rather
 * than a second copy of it. It used to be a copy, and a copy is what the
 * router's own list turned out to be too: sync was added to every list but
 * that one, and the row led to the not-found page.
 */
export type ScopedPanelView = PanelView;
export type RootInstallationView = (typeof ROOT_INSTALLATION_VIEWS)[number];
export type PersonalView = (typeof PERSONAL_VIEWS)[number];
/** History's two tables are addressable, so a reload lands where you left off. */
export type HistorySection = (typeof HISTORY_SECTIONS)[number];
export type RootRuntimeSection = (typeof ROOT_RUNTIME_SECTIONS)[number];
export type RootSection = 'overview' | 'queue' | 'installations' | 'access' | 'history' | 'runtime';
export type PanelSection = Exclude<ScopedPanelView, 'users' | 'invitations'> | 'access';
export type RootRoute =
  | {
      rootView: 'overview' | 'installations' | 'access-users' | 'access-invitations';
      dialog?: RouteDialog;
    }
  | {
      rootView:
        | 'history-audit'
        | 'history-failures'
        | 'runtime-settings'
        | 'runtime-service'
        | 'runtime-database';
    }
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
      view: RootInstallationView;
      section?: HistorySection;
      repository?: RepositoryPage;
      dialog?: RouteDialog;
    };

export type InstallationRoute = {
  account: string;
  view: ScopedPanelView;
  section?: HistorySection;
  /**
   * One repository's own page, which is a place inside the repositories view
   * rather than something standing over it - the navigation still reads
   * Repositories, and leaving the page returns to the list.
   */
  repository?: RepositoryPage;
  /** The sync view's open section; absent means the overview. */
  sync?: SyncSection;
  /**
   * One ruleset's own page, named the way a person names it. Only ever
   * present with `sync === 'rulesets'` - the list is the level above it.
   */
  syncRuleset?: string;
  /**
   * One template's own page, addressed by the path itself - slashes and
   * all. Only ever present with `sync === 'files'`.
   */
  syncFile?: string;
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

  const [namespace, encodedAccount, rawSection] = parts;
  if (
    namespace !== 'i' ||
    encodedAccount === undefined ||
    encodedAccount.length === 0 ||
    rawSection === undefined
  )
    return null;

  /* Access is a sidebar section, not a view pretending to be one. The panel's
     route vocabulary still uses the leaf names because that is what the
     components render, while the address carries the hierarchy people see. */
  const accessView =
    rawSection === 'access' ? ACCESS_SECTIONS.find((section) => section === parts[3]) : undefined;
  if (rawSection === 'access' && parts.length > 3 && accessView === undefined) return null;
  if (rawSection !== 'access' && !DIRECT_PANEL_VIEWS.some((view) => view === rawSection)) {
    return null;
  }
  const rawView = rawSection === 'access' ? (accessView ?? 'users') : rawSection;
  if (!isScopedPanelView(rawView)) return null;

  /* Everything past the view is history's table, a repository's page, or a
     dialog. Each view takes exactly one of the three, so the grammars can never
     be confused for each other. */
  const trailing = parts.slice(rawSection === 'access' ? 4 : 3);
  const repository = parseTrailingRepository(rawView, trailing);
  if (repository === 'invalid') return null;

  const dialog = repository === undefined ? parseTrailingDialog(rawView, trailing) : undefined;
  if (dialog === 'invalid') return null;

  const consumed = repository !== undefined || dialog !== undefined;
  const sync = parseTrailingSync(rawView, consumed ? [] : trailing);
  if (sync === 'invalid') return null;

  const section = parseSection(rawView, consumed ? undefined : trailing[0]);
  if (section === 'invalid' || (!consumed && rawView !== 'sync' && trailing.length > 1)) {
    return null;
  }

  let account: string;
  try {
    account = decodeURIComponent(encodedAccount);
  } catch {
    return null;
  }

  if (account.trim() === '') return null;
  const route: InstallationRoute = { account, view: rawView };
  if (repository !== undefined) return { ...route, repository };
  if (dialog !== undefined) return { ...route, dialog };
  if (sync !== undefined) return { ...route, ...sync };
  return section === undefined ? route : { ...route, section };
}

/**
 * Reads the segments past the repositories view as one repository's page.
 *
 * `undefined` for any other view or a path with nothing after it; `'invalid'`
 * for segments that were meant to be a repository and are not, which is an
 * address that does not resolve - a mistyped pane should say so rather than
 * quietly opening the file pane of a repository whose name was read as one.
 */
function parseTrailingRepository(
  view: string,
  segments: string[],
): RepositoryPage | undefined | 'invalid' {
  if (segments.length === 0 || view !== 'repositories') return undefined;
  if (segments.length > 2) return 'invalid';

  const [encodedName, rawSection] = segments;
  if (encodedName === undefined || encodedName === '') return 'invalid';

  let name: string;
  try {
    name = decodeURIComponent(encodedName);
  } catch {
    return 'invalid';
  }
  if (name.trim() === '') return 'invalid';
  /* A name is only ever read in the first position, so the repository called
     `behavior` is reachable, and `.../behavior/behavior` is its Behavior pane. */
  if (rawSection === undefined) return { name, section: 'file' };

  const section = REPOSITORY_SECTIONS.find((known) => known === rawSection);

  return section === undefined ? 'invalid' : { name, section };
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

  return dialogFromPath(view, segments) ?? 'invalid';
}

/**
 * Reads a dialog out of raw pathname segments.
 *
 * Everything in this module holds a pathname the router never saw, so the segments
 * arrive encoded and are decoded here. `parseDialogSegments` takes them decoded, which
 * is how the router hands its own over - decoding there as well would do it twice.
 */
function dialogFromPath(host: DialogHost, segments: string[]): RouteDialog | null {
  const decoded = decodeSegments(segments);

  return decoded === null ? null : parseDialogSegments(host, decoded);
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

  const view = requested?.view ?? 'defaults';
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
  if (
    route.rootView === 'runtime-settings' ||
    route.rootView === 'runtime-service' ||
    route.rootView === 'runtime-database'
  )
    return 'runtime';
  return route.rootView;
}

export function rootSectionRoute(section: RootSection): RootRoute {
  if (section === 'access') return { rootView: 'access-users' };
  if (section === 'history') return { rootView: 'history-audit' };
  if (section === 'runtime') return { rootView: 'runtime-service' };
  return { rootView: section };
}

export function isScopedPanelView(value: string | undefined): value is ScopedPanelView {
  return PANEL_VIEWS.some((view) => view === value);
}

export function isRootInstallationView(value: string | undefined): value is RootInstallationView {
  return ROOT_INSTALLATION_VIEWS.some((view) => view === value);
}

function routeTitleSegments(route: PanelRoute): string[] {
  if ('personal' in route) return [route.personal];
  if ('rootView' in route && route.rootView !== 'installation') {
    return route.rootView.split('-').reverse();
  }
  const view = route.view;
  const section = panelViewSection(view);
  const leaf = route.section ?? ('sync' in route ? route.sync : undefined) ?? view;
  return leaf === section ? [leaf] : [leaf, section];
}

/** `undefined` for "no segment", `'invalid'` for a segment that cannot be one. */
function parseSection(
  view: string,
  raw: string | undefined,
): HistorySection | undefined | 'invalid' {
  if (raw === undefined) return undefined;
  /* Sync's trailing segment is its own grammar, answered by `parseSyncSection`. */
  if (view === 'sync') return undefined;
  if (view !== 'history') return 'invalid';
  return HISTORY_SECTIONS.find((section) => section === raw) ?? 'invalid';
}

/**
 * Reads the segments past the sync view: a section, or a section and the one
 * object page it lists - a ruleset's name after `rulesets`.
 *
 * `undefined` for "no segment" (the overview); `'invalid'` for anything that
 * was meant to be sync's grammar and is not, which is an address that does
 * not resolve.
 */
function parseTrailingSync(
  view: string,
  segments: string[],
): Pick<InstallationRoute, 'sync' | 'syncRuleset' | 'syncFile'> | undefined | 'invalid' {
  if (view !== 'sync' || segments.length === 0) return undefined;

  const [rawSection, ...encodedRest] = segments;
  const sync = WRITTEN_SYNC_SECTIONS.find((section) => section === rawSection);
  if (sync === undefined) return 'invalid';
  if (encodedRest.length === 0) return { sync };
  /* Only the two object sections list named things. A ruleset's name is one
     segment; a file's path is as many as it carries slashes. */
  if (sync !== 'rulesets' && sync !== 'files') return 'invalid';
  if (sync === 'rulesets' && encodedRest.length > 1) return 'invalid';

  let parts: string[];
  try {
    parts = encodedRest.map((segment) => decodeURIComponent(segment));
  } catch {
    return 'invalid';
  }
  if (parts.some((part) => part.trim() === '')) return 'invalid';

  return sync === 'rulesets'
    ? { sync, syncRuleset: parts[0] ?? '' }
    : { sync, syncFile: parts.join('/') };
}

function parseRootRoute(parts: string[]): RootRoute | null {
  if (parts.length === 1) return { rootView: 'overview' };
  if (parts.length === 2 && parts[1] === 'installations') return { rootView: 'installations' };
  if (parts.length === 2 && parts[1] === 'runtime') return { rootView: 'runtime-service' };
  if (parts.length === 3 && parts[1] === 'runtime') {
    if (parts[2] === 'settings') return { rootView: 'runtime-settings' };
    if (parts[2] === 'service') return { rootView: 'runtime-service' };
    if (parts[2] === 'database') return { rootView: 'runtime-database' };
    return null;
  }
  if (parts.length >= 3 && parts[1] === 'access') {
    /* The Root console's tables take the same dialog grammar as an
       installation's, because they list the same things. */
    const host = parts[2] === 'users' ? 'access-users' : 'access-invitations';
    if (parts[2] === 'users' || parts[2] === 'invitations') {
      if (parts.length === 3) return { rootView: host };
      const dialog = dialogFromPath(host, parts.slice(3));

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
  if (parts.length < 4 || parts[1] !== 'installations') return null;

  const rawView = parts[3] ?? '';
  const accessView =
    rawView === 'access' ? ACCESS_SECTIONS.find((section) => section === parts[4]) : undefined;
  if (rawView === 'access' && parts.length > 4 && accessView === undefined) return null;
  if (rawView !== 'access' && !DIRECT_ROOT_INSTALLATION_VIEWS.some((view) => view === rawView)) {
    return null;
  }
  const view = rawView === 'access' ? (accessView ?? 'users') : rawView;
  if (!isRootInstallationView(view)) return null;
  const trailing = parts.slice(rawView === 'access' ? 5 : 4);
  const repository = parseTrailingRepository(view, trailing);
  if (repository === 'invalid') return null;

  const dialog = repository === undefined ? parseTrailingDialog(view, trailing) : undefined;
  if (dialog === 'invalid') return null;

  const consumed = repository !== undefined || dialog !== undefined;
  const section = parseSection(view, consumed ? undefined : trailing[0]);
  if (section === 'invalid' || (!consumed && trailing.length > 1)) return null;

  let account: string;
  try {
    account = decodeURIComponent(parts[2] ?? '');
  } catch {
    return null;
  }
  if (account.trim() === '') return null;
  const route: RootRoute = { rootView: 'installation', account, view };
  if (repository !== undefined) return { ...route, repository };
  if (dialog !== undefined) return { ...route, dialog };
  return section === undefined ? route : { ...route, section };
}

function findAccount(accounts: readonly string[], requested: string | null): string | undefined {
  if (requested === null) return undefined;
  const folded = requested.toLowerCase();
  return accounts.find((account) => account.toLowerCase() === folded);
}

import { normalizeBasePath } from './base.ts';
import {
  decodeSegments,
  isDialogHost,
  parseDialogSegments,
  type DialogHost,
  type RouteDialog,
} from './route-dialogs.ts';

export type { RouteDialog };

/**
 * A workspace has no Schedules page. Timing is the service's to set, so what a workspace
 * has is one row on its settings page saying when Smyklot acts and the way to ask for
 * that to change; the tables of policies and profiles are the operators' own and stay on
 * the console. It was a page that answered a question a workspace never asks.
 */
export const PANEL_VIEWS = [
  /* Where a workspace OPENS - what needs somebody, what is in flight, what
     just happened. It is the bare address rather than a written one, the way
     the console's own overview is: a workspace that opened on its settings
     page asked a reader to start from the least urgent thing it holds. */
  'overview',
  'settings',
  'repositories',
  'sync',
  'queue',
  'users',
  'invitations',
  'history',
] as const;

/** Views written directly after a workspace account in the route tree. */
export const DIRECT_PANEL_VIEWS = ['settings', 'repositories', 'sync', 'history'] as const;

/**
 * The views that belong to the reader rather than to a workspace or the console.
 *
 * Everything else the panel shows is scoped to something in the address: a
 * workspace under `/workspace/<account>`, the application under `/root`. The inbox is
 * scoped to whoever is signed in, which no path segment can name, so it sits at
 * the top: `/inbox`. One address, and the same page whichever part of the panel
 * it was reached from - a Root who opens it leaves the console rather than
 * carrying it along, because an address that says nothing about the console
 * cannot be reloaded back into one.
 */
export const PERSONAL_VIEWS = ['inbox', 'search'] as const;

/**
 * The views the Root console renders for one workspace, which is not every
 * view a workspace has.
 *
 * Sync is the difference. Configuring what an organization's repositories
 * should carry is the workspace's own business, reached by elevating into
 * it, and the console reads through its own Root-scoped endpoints rather than
 * the ones a workspace's members use. Accepting the address without
 * rendering anything is worse than refusing it: a bookmark that answers "this
 * view is unavailable" looks like a fault rather than a boundary.
 */
export const ROOT_WORKSPACE_VIEWS = [
  'settings',
  'repositories',
  'users',
  'invitations',
  'history',
] as const;

/** Root workspace views written directly after the workspace account. */
export const DIRECT_ROOT_WORKSPACE_VIEWS = ['settings', 'repositories', 'history'] as const;

export const HISTORY_SECTIONS = ['audit', 'failures'] as const;

/**
 * Queue sections are pages. Active owns the bare Queue address.
 *
 * Five, because the queue's own control offers five: which slice of the work a reader
 * is looking at is a fact about the page, and a fact about the page is an address here.
 * Two of them being links and three being state a reload forgets is the incoherence
 * this list exists to refuse.
 */
export const WRITTEN_QUEUE_SECTIONS = ['approvals', 'waiting', 'running', 'history'] as const;
export const QUEUE_SECTIONS = ['active', ...WRITTEN_QUEUE_SECTIONS] as const;
export type QueueSection = (typeof QUEUE_SECTIONS)[number];
export type WrittenQueueSection = (typeof WRITTEN_QUEUE_SECTIONS)[number];

/** The tables the Root console's access page is split into. */
export const ACCESS_SECTIONS = ['users', 'invitations'] as const;

/** The two addressable pages nested under Root Runtime. */
export const ROOT_RUNTIME_SECTIONS = ['service', 'settings'] as const;

/**
 * The sections of the sync view, each a sidebar row and an address.
 *
 * `overview` is not written into the path: it is where the view opens, so the
 * bare `/workspace/acme/sync` already means it.
 */
export const WRITTEN_SYNC_SECTIONS = ['labels', 'settings', 'rulesets', 'files', 'plan'] as const;
export const SYNC_SECTIONS = ['overview', ...WRITTEN_SYNC_SECTIONS] as const;
export type SyncSection = (typeof SYNC_SECTIONS)[number];

/**
 * What each sync section is CALLED, which is not what it is addressed by.
 *
 * `overview` is written "Sync status" and `settings` is written "Repository
 * options" - the tree already carries a Workspace settings row, and no two
 * rows in it may share a word a reader navigates by. Held here because the
 * tree, the overview board, the plan and the browser tab all say it, and a
 * word spelled in four places is a word that drifts in three.
 */
export const SYNC_SECTION_LABELS: Record<SyncSection, string> = {
  overview: 'Sync status',
  labels: 'Labels',
  settings: 'Repository options',
  rulesets: 'Rulesets',
  files: 'Shared files',
  plan: 'Plan',
};

/** One repository's own page. */
export interface RepositoryPage {
  /** Named the way a person names it - `api-gateway`, never an id. */
  name: string;
}

export type PanelView = (typeof PANEL_VIEWS)[number];

/**
 * A view in a workspace's own address, which is every view there is.
 *
 * The name is kept because it says which surface an address belongs to - the
 * console's subset is `RootWorkspaceView` - but it is the same list rather
 * than a second copy of it. It used to be a copy, and a copy is what the
 * router's own list turned out to be too: sync was added to every list but
 * that one, and the row led to the not-found page.
 */
export type ScopedPanelView = PanelView;
export type RootWorkspaceView = (typeof ROOT_WORKSPACE_VIEWS)[number];
export type PersonalView = (typeof PERSONAL_VIEWS)[number];
/** History's two tables are addressable, so a reload lands where you left off. */
export type HistorySection = (typeof HISTORY_SECTIONS)[number];
export type RootRuntimeSection = (typeof ROOT_RUNTIME_SECTIONS)[number];
export type RootSection =
  'overview' | 'queue' | 'schedules' | 'workspaces' | 'access' | 'history' | 'runtime';
export type PanelSection = Exclude<ScopedPanelView, 'users' | 'invitations'> | 'access';
export type RootRoute =
  | {
      rootView: 'overview' | 'workspaces' | 'schedules' | 'access-users' | 'access-invitations';
      dialog?: RouteDialog;
    }
  | {
      rootView:
        | 'history-audit'
        | 'history-failures'
        | 'queue-approvals'
        | 'queue-waiting'
        | 'queue-running'
        | 'queue-history'
        | 'runtime-settings'
        | 'runtime-service';
    }
  /**
   * Work the service has accepted and will do later, on a schedule it chooses.
   *
   * One word, like every other item in this nav, and a generic one: what is in it
   * today is pull requests waiting on their checks, and the data model already
   * carries cleanup retries. "Merge queue" is GitHub's name for batching pull
   * requests and testing them together, which is not this.
   */
  /* No `queue-recent`. The console kept a second surface for finished work - a table
     of its own at `/root/queue/recent` - and nothing on any page reached it: the
     queue's segments name Active, Needs a decision, Waiting, Running and Done, and
     Done is a card on the queue itself. An address a reader cannot arrive at is not a
     page, and the table, its story and the two sweeps that held it to the shared
     heading went with it. */
  | { rootView: 'queue' }
  /** A request is a page rather than a dialog: the timeline is a record to link someone to. */
  | { rootView: 'queue-request'; request: string }
  | {
      rootView: 'workspace';
      account: string;
      view: RootWorkspaceView;
      section?: HistorySection;
      repository?: RepositoryPage;
      dialog?: RouteDialog;
    };

export type WorkspaceRoute = {
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
  /** The Queue page the address names; absent means Active. */
  queue?: QueueSection;
  /** What is open on top of the view; see `route-dialogs`. */
  dialog?: RouteDialog;
};
/** A page of the reader's own, standing beside the workspaces and the console. */
export type PersonalRoute = { personal: PersonalView };
export type PanelRoute = WorkspaceRoute | RootRoute | PersonalRoute;

export interface ResolvedPanelRoute {
  account: string;
  view: PanelView;
  section?: HistorySection;
  queue?: QueueSection;
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
  /* A workspace with nothing after it IS its overview - the page it opens on
     takes the bare address rather than a written one, the way the console's
     own overview takes `/root`. */
  if (parts.length === 2 && parts[0] === 'workspace' && (parts[1] ?? '') !== '') {
    return { account: decodeURIComponent(parts[1] ?? ''), view: 'overview' };
  }
  if (parts.length < 3) return null;

  const [namespace, encodedAccount, rawSection] = parts;
  if (
    namespace !== 'workspace' ||
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
  if (
    rawSection !== 'access' &&
    rawSection !== 'queue' &&
    !DIRECT_PANEL_VIEWS.some((view) => view === rawSection)
  ) {
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

  const queue = parseTrailingQueue(rawView, consumed ? [] : trailing);
  if (queue === 'invalid') return null;

  const section = parseSection(rawView, consumed || queue !== undefined ? undefined : trailing[0]);
  if (
    section === 'invalid' ||
    (!consumed && rawView !== 'sync' && rawView !== 'queue' && trailing.length > 1)
  ) {
    return null;
  }

  let account: string;
  try {
    account = decodeURIComponent(encodedAccount);
  } catch {
    return null;
  }

  if (account.trim() === '') return null;
  const route: WorkspaceRoute = { account, view: rawView };
  if (repository !== undefined) return { ...route, repository };
  if (dialog !== undefined) return { ...route, dialog };
  if (sync !== undefined) return { ...route, ...sync };
  if (queue !== undefined) return { ...route, queue };
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
  /* The repository IS the whole address now. Its page used to open on one of five
     panes and carry the pane in a second segment; the page is one scroll, so there is
     no pane to name and an address that names one resolves to nothing. */
  if (segments.length > 1) return 'invalid';

  const [encodedName] = segments;
  if (encodedName === undefined || encodedName === '') return 'invalid';

  let name: string;
  try {
    name = decodeURIComponent(encodedName);
  } catch {
    return 'invalid';
  }

  return name.trim() === '' ? 'invalid' : { name };
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
  const object = routeTitleObject(route);
  const said = segments.map(routeSegmentLabel);
  if (object !== null) said.unshift(object);
  return [...said, 'SMYKLOT'].join(' | ');
}

export function panelViewSection(view: ScopedPanelView): PanelSection {
  return view === 'users' || view === 'invitations' ? 'access' : view;
}

/**
 * Where a segment is not simply its own word capitalised.
 *
 * The tab says what the sidebar row and the heading say, which for the settings pages
 * is more than the segment: the tree carries a Workspace settings row and a Service
 * settings row, so a tab reading "Settings" says which of the two only by luck. The
 * page used to be addressed `defaults` and the tab read "Defaults", which is a word the
 * dictionary retires and the only place in the panel it still reached a reader.
 */
const SEGMENT_LABELS: Record<string, string> = {
  settings: 'Workspace settings',
  /* Two words apiece, and only the first is capitalised: spelling these from the
     segment gave "Service Health", which is the sidebar's row in title case. */
  'service-health': 'Service health',
  'service-settings': 'Service settings',
};

export function routeSegmentLabel(segment: string): string {
  return (
    SEGMENT_LABELS[segment] ??
    segment
      .split('-')
      .map((word) => word.slice(0, 1).toUpperCase() + word.slice(1))
      .join(' ')
  );
}

export function resolvePanelRoute(
  availableAccounts: readonly string[],
  requested: WorkspaceRoute | null,
  preferredAccount: string | null,
): ResolvedPanelRoute | null {
  const requestedAccount = findAccount(availableAccounts, requested?.account ?? null);
  const account =
    requestedAccount ?? findAccount(availableAccounts, preferredAccount) ?? availableAccounts[0];
  if (account === undefined) return null;

  const view = requested?.view ?? 'overview';
  /* History always resolves to a named table, so the address bar never sits on
     a bare /history that a reload would have to guess at. */
  return view === 'history'
    ? { account, view, section: requested?.section ?? 'audit' }
    : view === 'queue'
      ? { account, view, queue: requested?.queue ?? 'active' }
      : { account, view };
}

/** Whether a console view is one of the queue's own sections, written as its address. */
function isQueueSectionView(view: RootRoute['rootView']): view is `queue-${WrittenQueueSection}` {
  return WRITTEN_QUEUE_SECTIONS.some((section) => view === `queue-${section}`);
}

export function rootSection(route: RootRoute): RootSection {
  if (route.rootView === 'access-users' || route.rootView === 'access-invitations') return 'access';
  if (route.rootView === 'history-audit' || route.rootView === 'history-failures') return 'history';
  if (route.rootView === 'workspace') return 'workspaces';
  if (route.rootView === 'queue-request') return 'queue';
  if (isQueueSectionView(route.rootView)) return 'queue';
  if (route.rootView === 'runtime-settings' || route.rootView === 'runtime-service') {
    return 'runtime';
  }
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

export function isRootWorkspaceView(value: string | undefined): value is RootWorkspaceView {
  return ROOT_WORKSPACE_VIEWS.some((view) => view === value);
}

/**
 * Console views the product does not call what the address calls them.
 *
 * `workspaces` used to be here, because the address said GitHub's word for a grant
 * while the console, the tree and the page all said Workspaces. The address says
 * Workspaces now too: one word for one thing, everywhere a reader can see it, which is
 * worth more than the links an operator kept to the old one.
 */
const ROOT_VIEW_WORDS: Partial<Record<RootRoute['rootView'], string>> = {
  /* "Runtime" is the address's word for the two pages under it, and neither page
     says it: this one is Service health and the other is Service settings. */
  'runtime-service': 'service-health',
  'runtime-settings': 'service-settings',
};

function routeTitleSegments(route: PanelRoute): string[] {
  if ('personal' in route) return [route.personal];
  if ('rootView' in route && route.rootView !== 'workspace') {
    const said = ROOT_VIEW_WORDS[route.rootView];
    if (said !== undefined) return [said];

    return route.rootView.split('-').reverse();
  }
  const view = route.view;
  const section = panelViewSection(view);
  const sync = 'sync' in route ? route.sync : undefined;
  const leaf = route.section ?? sync ?? ('queue' in route ? route.queue : undefined) ?? view;
  /* A sync section is named rather than spelled from its address: the tab
     would otherwise read "Settings" on the page whose title is Repository
     options, which is the one word the tree deliberately does not use twice. */
  const said = sync !== undefined && leaf === sync ? SYNC_SECTION_LABELS[sync] : leaf;
  return leaf === section ? [said] : [said, section];
}

/**
 * The object one page is about, for the tab that would otherwise name only the
 * list it came from - two ruleset tabs both reading "Rulesets" are two tabs a
 * reader has to open to tell apart. Returned unspelled, because a name Smyklot
 * did not choose is not a segment to capitalise.
 */
function routeTitleObject(route: PanelRoute): string | null {
  if ('personal' in route || 'rootView' in route) return null;
  return route.syncRuleset ?? route.syncFile ?? route.repository?.name ?? null;
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
): Pick<WorkspaceRoute, 'sync' | 'syncRuleset' | 'syncFile'> | undefined | 'invalid' {
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

/** Reads the one Queue section written after the bare Active page. */
function parseTrailingQueue(
  view: string,
  segments: string[],
): QueueSection | undefined | 'invalid' {
  if (view !== 'queue' || segments.length === 0) return undefined;
  if (segments.length !== 1) return 'invalid';
  return WRITTEN_QUEUE_SECTIONS.find((section) => section === segments[0]) ?? 'invalid';
}

function parseRootRoute(parts: string[]): RootRoute | null {
  if (parts.length === 1) return { rootView: 'overview' };
  if (parts.length === 2 && parts[1] === 'workspaces') return { rootView: 'workspaces' };
  if (parts.length === 2 && parts[1] === 'runtime') return { rootView: 'runtime-service' };
  if (parts.length === 3 && parts[1] === 'runtime') {
    if (parts[2] === 'settings') return { rootView: 'runtime-settings' };
    if (parts[2] === 'service') return { rootView: 'runtime-service' };
    /* The database's own page is part of Service health now. The address still
       resolves - a route redirects it - so this answers with the page it lands on
       rather than with a 404. */
    if (parts[2] === 'database') return { rootView: 'runtime-service' };
    return null;
  }
  if (parts.length >= 3 && parts[1] === 'access') {
    /* The Root console's tables take the same dialog grammar as a
       workspace's, because they list the same things. */
    const host = parts[2] === 'users' ? 'access-users' : 'access-invitations';
    if (parts[2] === 'users' || parts[2] === 'invitations') {
      if (parts.length === 3) return { rootView: host };
      const dialog = dialogFromPath(host, parts.slice(3));

      return dialog === null ? null : { rootView: host, dialog };
    }
  }
  if (parts.length === 2 && parts[1] === 'queue') return { rootView: 'queue' };
  if (parts.length === 2 && parts[1] === 'schedules') return { rootView: 'schedules' };
  if (parts.length === 3 && parts[1] === 'queue') {
    const written = WRITTEN_QUEUE_SECTIONS.find((section) => section === parts[2]);
    if (written !== undefined) return { rootView: `queue-${written}` };
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
     through to the workspace default. */
  if (parts.length === 2 && parts[1] === 'history') return { rootView: 'history-audit' };
  if (parts.length === 2 && parts[1] === 'access') return { rootView: 'access-users' };
  if (parts.length < 4 || parts[1] !== 'workspaces') return null;

  const rawView = parts[3] ?? '';
  const accessView =
    rawView === 'access' ? ACCESS_SECTIONS.find((section) => section === parts[4]) : undefined;
  if (rawView === 'access' && parts.length > 4 && accessView === undefined) return null;
  if (rawView !== 'access' && !DIRECT_ROOT_WORKSPACE_VIEWS.some((view) => view === rawView)) {
    return null;
  }
  const view = rawView === 'access' ? (accessView ?? 'users') : rawView;
  if (!isRootWorkspaceView(view)) return null;
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
  const route: RootRoute = { rootView: 'workspace', account, view };
  if (repository !== undefined) return { ...route, repository };
  if (dialog !== undefined) return { ...route, dialog };
  return section === undefined ? route : { ...route, section };
}

function findAccount(accounts: readonly string[], requested: string | null): string | undefined {
  if (requested === null) return undefined;
  const folded = requested.toLowerCase();
  return accounts.find((account) => account.toLowerCase() === folded);
}

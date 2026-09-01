import { resolve } from '$app/paths';
import type { RouteId } from '$app/types';

import {
  dialogSegments,
  isDialogHost,
  parseDialogSegments,
  type DialogHost,
  type RouteDialog,
} from './route-dialogs.ts';
import {
  HISTORY_SECTIONS,
  WRITTEN_QUEUE_SECTIONS,
  WRITTEN_SYNC_SECTIONS,
  isScopedPanelView,
  isRootWorkspaceView,
  type HistorySection,
  type PanelRoute,
  type RepositoryPage,
  type RootRoute,
  type SyncSection,
  type WrittenQueueSection,
} from './routes.ts';

/**
 * Where a route lives, resolved by SvelteKit against its own route tree.
 *
 * Every branch names a route id as a literal, and those literals are checked: they come
 * from `$app/types`, which the build generates from `src/routes`. A route that is
 * renamed or removed stops compiling here rather than answering 404 at a link somebody
 * clicks, and a parameter that is missing or misspelled is a type error rather than a
 * segment that quietly reads `undefined`.
 *
 * The base path is SvelteKit's to add. Nothing here threads one, which is why these
 * return an address ready to navigate to or to put in an `href`.
 *
 * The panel keeps its own route vocabulary - `PanelRoute` says what is being looked at,
 * not where it is written down - so this is the one place the two meet.
 */
/**
 * The search results page, carrying the query in the address.
 *
 * A query lives in the address rather than in the page, so a reader who opens a result
 * and presses back is given the search they left, and one who sends the link sends the
 * search rather than an empty field. `resolve` answers the path; the query string is
 * this function's, because a route id has no room for one.
 */
export function searchAddress(query: string): string {
  const path = resolve('/search');
  const asked = query.trim();

  return asked === '' ? path : `${path}?q=${encodeURIComponent(asked)}`;
}

export function panelAddress(route: PanelRoute): string {
  if ('rootView' in route) return rootAddress(route);
  if ('personal' in route) {
    return route.personal === 'search' ? resolve('/search') : resolve('/inbox');
  }

  const account = encodeURIComponent(route.account);
  if (route.view === 'users' || route.view === 'invitations') {
    return resolve('/workspace/[account]/access/[section=accessSection]/[...rest=dialogPath]', {
      account,
      section: route.view,
      rest: route.dialog === undefined ? '' : (dialogRest(route.view, route.dialog) ?? ''),
    });
  }

  if (route.view === 'history') {
    return resolve('/workspace/[account]/history/[[section=historySection]]', {
      account,
      section: route.section,
    });
  }

  if (route.view === 'queue') {
    return route.queue === undefined || route.queue === 'active'
      ? resolve('/workspace/[account]/queue', { account })
      : resolve('/workspace/[account]/queue/[section=queueSection]', {
          account,
          section: route.queue,
        });
  }

  if (route.view === 'repositories' && named(route.repository)) {
    return resolve('/workspace/[account]/repositories/[repository]', {
      account,
      repository: encodeURIComponent(route.repository.name),
    });
  }

  /* One ruleset's own page, a level below its list. */
  if (route.view === 'sync' && route.sync === 'rulesets' && route.syncRuleset !== undefined) {
    return resolve('/workspace/[account]/sync/rulesets/[ruleset]', {
      account,
      ruleset: encodeURIComponent(route.syncRuleset),
    });
  }

  /* One template's own page - the path IS the address, slash for slash. */
  if (route.view === 'sync' && route.sync === 'files' && route.syncFile !== undefined) {
    return resolve('/workspace/[account]/sync/files/[...file=syncFilePath]', {
      account,
      file: route.syncFile.split('/').map(encodeURIComponent).join('/'),
    });
  }

  /* The overview is the bare view - an address that names it as well is one a
     reader would have to be told to ignore. */
  if (route.view === 'sync' && route.sync !== undefined && route.sync !== 'overview') {
    return resolve('/workspace/[account]/sync/[section=syncSection]', {
      account,
      section: route.sync,
    });
  }

  /* The workspace opens on its overview, so the overview IS the workspace's
     address. A second one naming it would be one a reader has to be told to
     ignore - the console's own overview is the bare `/root` for the same
     reason. */
  if (route.view === 'overview') return resolve('/workspace/[account]', { account });

  return resolve('/workspace/[account]/[view=panelView]', { account, view: route.view });
}

function rootAddress(route: RootRoute): string {
  switch (route.rootView) {
    case 'overview':
      return resolve('/root');
    case 'workspaces':
      return resolve('/root/workspaces');
    case 'queue':
      return resolve('/root/queue');
    case 'queue-approvals':
      return resolve('/root/queue/[section=queueSection]', { section: 'approvals' });
    case 'queue-waiting':
      return resolve('/root/queue/[section=queueSection]', { section: 'waiting' });
    case 'queue-running':
      return resolve('/root/queue/[section=queueSection]', { section: 'running' });
    case 'queue-history':
      return resolve('/root/queue/[section=queueSection]', { section: 'history' });
    case 'queue-request':
      return resolve('/root/queue/request/[id]', { id: encodeURIComponent(route.request) });
    case 'schedules':
      return resolve('/root/schedules');
    case 'runtime-settings':
      return resolve('/root/runtime/settings');
    case 'runtime-service':
      return resolve('/root/runtime/service');
    case 'history-audit':
    case 'history-failures':
      return resolve('/root/history/[[section=historySection]]', {
        section: route.rootView === 'history-audit' ? 'audit' : 'failures',
      });
    // `rootView` is the dialog host's own name here, so it names both the section the
    // address takes and the host any dialog standing on it belongs to.
    case 'access-users':
    case 'access-invitations':
      return resolve('/root/access/[section=accessSection]/[...rest=dialogPath]', {
        section: route.rootView === 'access-users' ? 'users' : 'invitations',
        rest: route.dialog === undefined ? '' : (dialogRest(route.rootView, route.dialog) ?? ''),
      });
    case 'workspace':
      return rootWorkspaceAddress(route);
  }
}

function rootWorkspaceAddress(route: RootRoute & { rootView: 'workspace' }): string {
  const account = encodeURIComponent(route.account);
  if (route.view === 'users' || route.view === 'invitations') {
    return resolve(
      '/root/workspaces/[account]/access/[section=accessSection]/[...rest=dialogPath]',
      {
        account,
        section: route.view,
        rest: route.dialog === undefined ? '' : (dialogRest(route.view, route.dialog) ?? ''),
      },
    );
  }

  if (route.view === 'history') {
    return resolve('/root/workspaces/[account]/history/[[section=historySection]]', {
      account,
      section: route.section,
    });
  }

  if (route.view === 'repositories' && named(route.repository)) {
    return resolve('/root/workspaces/[account]/repositories/[repository]', {
      account,
      repository: encodeURIComponent(route.repository.name),
    });
  }

  return resolve('/root/workspaces/[account]/[view=rootWorkspaceView]', {
    account,
    view: route.view,
  });
}

/**
 * Whether a route names one repository rather than the list.
 *
 * An unnamed one is the list: the panel carries a repository on the route it is a place
 * inside, and a blank name is that route with nothing chosen yet.
 */
function named(repository: RepositoryPage | undefined): repository is RepositoryPage {
  return repository !== undefined && repository.name !== '';
}

/**
 * The rest parameter a dialog contributes, or `null` when it names no address.
 *
 * `null` rather than `''`, because an empty rest is itself a real address - the host
 * view with no dialog open - and the caller answers that with a different route id.
 */
function dialogRest(host: DialogHost, dialog: RouteDialog): string | null {
  const segments = dialogSegments(host, dialog);
  if (segments === null) return null;

  return segments.map((segment) => encodeURIComponent(segment)).join('/');
}

/**
 * The route an address matched, read back into the panel's own vocabulary.
 *
 * The inverse of `panelAddress`, and the reason both live here: the panel says what is
 * being looked at, SvelteKit says where it is written down, and the translation belongs
 * in one place going each way. This reads what the router already decided - `page.route.id`
 * and `page.params` - rather than parsing the pathname a second time, so a renamed route
 * is a compile error on both sides instead of a silent mismatch on one.
 */
export function panelRouteAt(
  id: RouteId | null,
  params: Readonly<Record<string, string | undefined>>,
): PanelRoute | null {
  const account = params.account ?? '';
  const section = params.section;

  switch (id) {
    case '/inbox':
      return { personal: 'inbox' };
    case '/search':
      return { personal: 'search' };

    case '/workspace/[account]':
      return { account, view: 'overview' };
    case '/workspace/[account]/[view=panelView]':
      return withView(account, params.view);
    case '/workspace/[account]/queue':
      return { account, view: 'queue' };
    case '/workspace/[account]/queue/[section=queueSection]':
      return { account, view: 'queue', queue: writtenQueueSection(section) };
    case '/workspace/[account]/access':
      return withView(account, 'users');
    case '/workspace/[account]/access/[section=accessSection]/[...rest=dialogPath]':
      return withView(account, section, undefined, dialogAt(section, params.rest));
    case '/workspace/[account]/history/[[section=historySection]]':
      return withView(account, 'history', asSection(section));
    case '/workspace/[account]/sync/[section=syncSection]':
      return { account, view: 'sync', sync: asSyncSection(section) };
    case '/workspace/[account]/sync/rulesets/[ruleset]':
      return { account, view: 'sync', sync: 'rulesets', syncRuleset: params.ruleset ?? '' };
    case '/workspace/[account]/sync/files/[...file=syncFilePath]': {
      /* SvelteKit has already decoded every segment in the rest parameter.
         Decoding again turns a literal percent sequence into another name -
         or throws for a perfectly valid name such as `100%bad.json`. */
      const segments = (params.file ?? '').split('/');
      return {
        account,
        view: 'sync',
        sync: 'files',
        syncFile: segments.join('/'),
      };
    }
    case '/workspace/[account]/repositories/[repository]':
      return { account, view: 'repositories', repository: repositoryAt(params) };

    case '/root':
      return { rootView: 'overview' };
    case '/root/workspaces':
      return { rootView: 'workspaces' };
    case '/root/queue':
      return { rootView: 'queue' };
    case '/root/queue/[section=queueSection]':
      return { rootView: `queue-${writtenQueueSection(section)}` };
    case '/root/queue/request/[id]':
      return { rootView: 'queue-request', request: params.id ?? '' };
    case '/root/schedules':
      return { rootView: 'schedules' };
    case '/root/runtime':
      return { rootView: 'runtime-service' };
    case '/root/runtime/settings':
      return { rootView: 'runtime-settings' };
    case '/root/runtime/service':
      return { rootView: 'runtime-service' };
    /* The address survives its page: a redirect sends it to Service health, which
       is where the database now lives. Read back as the page it lands on. */
    case '/root/runtime/database':
      return { rootView: 'runtime-service' };
    case '/root/history/[[section=historySection]]':
      return { rootView: section === 'failures' ? 'history-failures' : 'history-audit' };

    // `/root/access` redirects to a section, so it only ever shows the first one.
    case '/root/access':
      return { rootView: 'access-users' };
    case '/root/access/[section=accessSection]/[...rest=dialogPath]': {
      const host = section === 'invitations' ? 'access-invitations' : 'access-users';

      return { rootView: host, dialog: dialogAt(host, params.rest) };
    }

    case '/root/workspaces/[account]/[view=rootWorkspaceView]':
      return rootWorkspace(account, params.view);
    case '/root/workspaces/[account]/access':
      return rootWorkspace(account, 'users');
    case '/root/workspaces/[account]/access/[section=accessSection]/[...rest=dialogPath]':
      return rootWorkspace(account, section, undefined, dialogAt(section, params.rest));
    case '/root/workspaces/[account]/history/[[section=historySection]]':
      return rootWorkspace(account, 'history', asSection(section));
    case '/root/workspaces/[account]/repositories/[repository]':
      return {
        rootView: 'workspace',
        account,
        view: 'repositories',
        repository: repositoryAt(params),
      };

    default:
      return null;
  }
}

function withView(
  account: string,
  view: string | undefined,
  section?: HistorySection,
  dialog?: RouteDialog,
): PanelRoute | null {
  return isScopedPanelView(view) ? { account, view, section, dialog } : null;
}

function rootWorkspace(
  account: string,
  view: string | undefined,
  section?: HistorySection,
  dialog?: RouteDialog,
): PanelRoute | null {
  return isRootWorkspaceView(view)
    ? { rootView: 'workspace', account, view, section, dialog }
    : null;
}

function asSection(value: string | undefined): HistorySection | undefined {
  return HISTORY_SECTIONS.find((section) => section === value);
}

/**
 * The queue section an address names.
 *
 * The matcher has already refused anything outside the list, so the fallback is for the
 * type rather than for a reader: Active is the section the bare Queue address is, and
 * the one a section that cannot be read should behave as.
 */
function writtenQueueSection(value: string | undefined): WrittenQueueSection {
  return WRITTEN_QUEUE_SECTIONS.find((section) => section === value) ?? 'approvals';
}

/** The written sections only: the overview never reaches this route. */
function asSyncSection(value: string | undefined): SyncSection | undefined {
  return WRITTEN_SYNC_SECTIONS.find((section) => section === value);
}

/**
 * The repository an address names.
 *
 * The name comes back the way the reader wrote it: the router has already decoded the
 * segment, which is the half `panelAddress` encoded.
 */
function repositoryAt(params: Readonly<Record<string, string | undefined>>): RepositoryPage {
  return { name: params.repository ?? '' };
}

/**
 * The dialog the trailing segments name, or none when they name no dialog this host has.
 *
 * A tail that names nothing does not make the address name nothing: it still names the
 * view underneath, which is what the chrome around the page should show. Whether the page
 * itself resolved is SvelteKit's answer, not this one - the load guard raises 404 and
 * `page.error` carries it, which is what `syncRouteContext` reads.
 */
function dialogAt(view: string | undefined, rest: string | undefined): RouteDialog | undefined {
  if (view === undefined || !isDialogHost(view)) return undefined;
  const segments = rest?.split('/').filter((segment) => segment !== '') ?? [];
  if (segments.length === 0) return undefined;

  return parseDialogSegments(view, segments) ?? undefined;
}

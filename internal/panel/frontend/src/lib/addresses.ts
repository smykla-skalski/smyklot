import { resolve } from '$app/paths';
import type { RouteId } from '$app/types';

import {
  decodeSegments,
  dialogSegments,
  isDialogHost,
  parseDialogSegments,
  type DialogHost,
  type RouteDialog,
} from './route-dialogs.ts';
import {
  HISTORY_SECTIONS,
  REPOSITORY_SECTIONS,
  WRITTEN_SYNC_SECTIONS,
  isScopedPanelView,
  isRootInstallationView,
  type HistorySection,
  type PanelRoute,
  type RepositoryPage,
  type RepositorySection,
  type RootRoute,
  type SyncSection,
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
export function panelAddress(route: PanelRoute): string {
  if ('rootView' in route) return rootAddress(route);
  if ('personal' in route) return resolve('/inbox');

  const account = encodeURIComponent(route.account);
  if (route.view === 'history') {
    return resolve('/i/[account]/history/[[section=historySection]]', {
      account,
      section: route.section,
    });
  }

  if (route.view === 'repositories' && named(route.repository)) {
    return resolve('/i/[account]/repositories/[repository]/[[section=repositorySection]]', {
      account,
      repository: encodeURIComponent(route.repository.name),
      section: writtenSection(route.repository),
    });
  }

  /* One ruleset's own page, a level below its list. */
  if (route.view === 'sync' && route.sync === 'rulesets' && route.syncRuleset !== undefined) {
    return resolve('/i/[account]/sync/rulesets/[ruleset]', {
      account,
      ruleset: encodeURIComponent(route.syncRuleset),
    });
  }

  /* One template's own page - the path IS the address, slash for slash. */
  if (route.view === 'sync' && route.sync === 'files' && route.syncFile !== undefined) {
    return resolve('/i/[account]/sync/files/[...file=syncFilePath]', {
      account,
      file: route.syncFile.split('/').map(encodeURIComponent).join('/'),
    });
  }

  /* The overview is the bare view - an address that names it as well is one a
     reader would have to be told to ignore. */
  if (route.view === 'sync' && route.sync !== undefined && route.sync !== 'overview') {
    return resolve('/i/[account]/sync/[section=syncSection]', {
      account,
      section: route.sync,
    });
  }

  if (route.dialog !== undefined && isDialogHost(route.view)) {
    const rest = dialogRest(route.view, route.dialog);
    if (rest !== null) {
      return resolve('/i/[account]/[view=dialogHostView]/[...rest=dialogPath]', {
        account,
        view: route.view,
        rest,
      });
    }
  }

  return resolve('/i/[account]/[view=panelView]', { account, view: route.view });
}

function rootAddress(route: RootRoute): string {
  switch (route.rootView) {
    case 'overview':
      return resolve('/root');
    case 'installations':
      return resolve('/root/installations');
    case 'queue':
      return resolve('/root/queue');
    case 'queue-recent':
      return resolve('/root/queue/recent');
    case 'queue-request':
      return resolve('/root/queue/request/[id]', { id: encodeURIComponent(route.request) });
    case 'settings':
      return resolve('/root/settings');
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
    case 'installation':
      return rootInstallationAddress(route);
  }
}

function rootInstallationAddress(route: RootRoute & { rootView: 'installation' }): string {
  const account = encodeURIComponent(route.account);
  if (route.view === 'history') {
    return resolve('/root/installations/[account]/history/[[section=historySection]]', {
      account,
      section: route.section,
    });
  }

  if (route.view === 'repositories' && named(route.repository)) {
    return resolve(
      '/root/installations/[account]/repositories/[repository]/[[section=repositorySection]]',
      {
        account,
        repository: encodeURIComponent(route.repository.name),
        section: writtenSection(route.repository),
      },
    );
  }

  if (route.dialog !== undefined && isDialogHost(route.view)) {
    const rest = dialogRest(route.view, route.dialog);
    if (rest !== null) {
      return resolve('/root/installations/[account]/[view=dialogHostView]/[...rest=dialogPath]', {
        account,
        view: route.view,
        rest,
      });
    }
  }

  return resolve('/root/installations/[account]/[view=rootInstallationView]', {
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
 * The pane an address spells out, which is none for the one the page opens on.
 *
 * `file` is where a repository starts, so the bare address already means it, and one
 * that says so as well is an address a reader would have to be told to ignore.
 */
function writtenSection(repository: RepositoryPage): RepositorySection | undefined {
  return repository.section === 'file' ? undefined : repository.section;
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

    case '/i/[account]/[view=panelView]':
      return withView(account, params.view);
    case '/i/[account]/[view=dialogHostView]/[...rest=dialogPath]':
      return withView(account, params.view, undefined, dialogAt(params.view, params.rest));
    case '/i/[account]/history/[[section=historySection]]':
      return withView(account, 'history', asSection(section));
    case '/i/[account]/sync/[section=syncSection]':
      return { account, view: 'sync', sync: asSyncSection(section) };
    case '/i/[account]/sync/rulesets/[ruleset]':
      return { account, view: 'sync', sync: 'rulesets', syncRuleset: params.ruleset ?? '' };
    case '/i/[account]/sync/files/[...file=syncFilePath]': {
      /* A rest parameter arrives raw, the way the dialog paths do - decoded
         segment by segment, so a path with an encoded character in a name
         comes back as the name. */
      const segments = decodeSegments((params.file ?? '').split('/'));
      return {
        account,
        view: 'sync',
        sync: 'files',
        syncFile: segments === null ? '' : segments.join('/'),
      };
    }
    case '/i/[account]/repositories/[repository]/[[section=repositorySection]]':
      return { account, view: 'repositories', repository: repositoryAt(params) };

    case '/root':
      return { rootView: 'overview' };
    case '/root/installations':
      return { rootView: 'installations' };
    case '/root/queue':
      return { rootView: 'queue' };
    case '/root/queue/recent':
      return { rootView: 'queue-recent' };
    case '/root/queue/request/[id]':
      return { rootView: 'queue-request', request: params.id ?? '' };
    case '/root/settings':
      return { rootView: 'settings' };
    case '/root/history/[[section=historySection]]':
      return { rootView: section === 'failures' ? 'history-failures' : 'history-audit' };

    // `/root/access` redirects to a section, so it only ever shows the first one.
    case '/root/access':
      return { rootView: 'access-users' };
    case '/root/access/[section=accessSection]/[...rest=dialogPath]': {
      const host = section === 'invitations' ? 'access-invitations' : 'access-users';

      return { rootView: host, dialog: dialogAt(host, params.rest) };
    }

    case '/root/installations/[account]/[view=rootInstallationView]':
      return rootInstallation(account, params.view);
    case '/root/installations/[account]/[view=dialogHostView]/[...rest=dialogPath]':
      return rootInstallation(account, params.view, undefined, dialogAt(params.view, params.rest));
    case '/root/installations/[account]/history/[[section=historySection]]':
      return rootInstallation(account, 'history', asSection(section));
    case '/root/installations/[account]/repositories/[repository]/[[section=repositorySection]]':
      return {
        rootView: 'installation',
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

function rootInstallation(
  account: string,
  view: string | undefined,
  section?: HistorySection,
  dialog?: RouteDialog,
): PanelRoute | null {
  return isRootInstallationView(view)
    ? { rootView: 'installation', account, view, section, dialog }
    : null;
}

function asSection(value: string | undefined): HistorySection | undefined {
  return HISTORY_SECTIONS.find((section) => section === value);
}

/** The written sections only: the overview never reaches this route. */
function asSyncSection(value: string | undefined): SyncSection | undefined {
  return WRITTEN_SYNC_SECTIONS.find((section) => section === value);
}

/**
 * The repository an address names, opened on the pane it spells out.
 *
 * The name comes back the way the reader wrote it: the router has already decoded the
 * segment, which is the half `panelAddress` encoded. A missing pane is the one the page
 * opens on rather than no pane at all - the address leaves `file` unwritten.
 */
function repositoryAt(params: Readonly<Record<string, string | undefined>>): RepositoryPage {
  return {
    name: params.repository ?? '',
    section: REPOSITORY_SECTIONS.find((pane) => pane === params.section) ?? 'file',
  };
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

import { resolve } from '$app/paths';

import {
  dialogSegments,
  isDialogHost,
  type DialogHost,
  type RouteDialog,
} from './route-dialogs.ts';
import type { PanelRoute, RootRoute } from './routes.ts';

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

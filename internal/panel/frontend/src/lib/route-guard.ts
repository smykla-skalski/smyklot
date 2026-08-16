import { error, redirect } from '@sveltejs/kit';

import { isDialogHost, parseDialogSegments } from './route-dialogs.ts';

export function guardPanelViewRest(view: string, rest: string | undefined, pathname: string): void {
  const segments = rest?.split('/').filter((segment) => segment !== '') ?? [];

  if (view === 'history') {
    if (segments.length === 0) redirect(307, `${pathname.replace(/\/$/u, '')}/audit`);
    if (segments.length === 1 && (segments[0] === 'audit' || segments[0] === 'failures')) return;
    error(404, 'History view not found');
  }

  if (segments.length === 0) return;
  if (isDialogHost(view) && parseDialogSegments(view, segments) !== null) return;
  error(404, 'Panel view not found');
}

export function guardRootAccessRest(section: string, rest: string | undefined): void {
  const segments = rest?.split('/').filter((segment) => segment !== '') ?? [];
  if (segments.length === 0) return;

  const host = section === 'users' ? 'access-users' : 'access-invitations';
  if (parseDialogSegments(host, segments) !== null) return;
  error(404, 'Root access view not found');
}

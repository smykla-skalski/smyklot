import { error, redirect } from '@sveltejs/kit';

import { isDialogHost, parseDialogSegments } from './route-dialogs.ts';

/**
 * What the routes cannot say, said here.
 *
 * The route tree carries the shape of an address - which views take a dialog,
 * which take a section, and how deep either may go - so the server refuses the
 * rest from the generated manifest before the panel has loaded. What is left is
 * whether the segments are a dialog this host actually knows, which depends on
 * names people chose and can only be read here.
 */
export function guardDialogRest(view: string, rest: string | undefined): void {
  const segments = rest?.split('/').filter((segment) => segment !== '') ?? [];
  if (segments.length === 0) return;
  if (isDialogHost(view) && parseDialogSegments(view, segments) !== null) return;

  error(404, 'Panel view not found');
}

/** History opens on its first table when the address does not name one. */
export function guardHistorySection(section: string | undefined, pathname: string): void {
  if (section !== undefined) return;

  redirect(307, `${pathname.replace(/\/$/u, '')}/audit`);
}

export function guardRootAccessRest(section: string, rest: string | undefined): void {
  const segments = rest?.split('/').filter((segment) => segment !== '') ?? [];
  if (segments.length === 0) return;

  const host = section === 'users' ? 'access-users' : 'access-invitations';
  if (parseDialogSegments(host, segments) !== null) return;
  error(404, 'Root access view not found');
}

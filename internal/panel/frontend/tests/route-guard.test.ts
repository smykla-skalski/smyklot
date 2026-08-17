import { describe, expect, it } from 'vitest';

import {
  guardDialogRest,
  guardHistorySection,
  guardRootAccessRest,
} from '../src/lib/route-guard.ts';

/**
 * What the guards are left with, now that the routes carry the rest.
 *
 * A view that hosts no dialog has no route with anything after it, and history's
 * section is matched by the route rather than read here - so those addresses no
 * longer reach a load function at all, and the server refuses them from the
 * generated manifest before the panel loads. `server_test.go` is where that is
 * checked. What is still this side's to decide is whether the segments name a
 * dialog the host actually knows, because that is made of names people chose.
 */
describe('panel dialog route guard', () => {
  it('accepts a host with no dialog open and the dialogs it knows', () => {
    expect(() => guardDialogRest('repositories', undefined)).not.toThrow();
    expect(() => guardDialogRest('repositories', 'api-gateway/commands')).not.toThrow();
    expect(() => guardDialogRest('users', 'octocat/history')).not.toThrow();
  });

  it('rejects segments the host cannot open', () => {
    expect(() => guardDialogRest('repositories', 'api-gateway/unknown')).toThrow();
    expect(() => guardDialogRest('users', 'octocat/not-an-action')).toThrow();
  });

  /* The route already refuses these, so the guard is never asked. It answers
     the same way regardless, because a guard that depends on its caller having
     checked first is a guard that stops being true when the caller moves. */
  it('rejects a view that hosts no dialog', () => {
    expect(() => guardDialogRest('settings', 'extra')).toThrow();
    expect(() => guardDialogRest('history', 'unknown')).toThrow();
  });
});

describe('history section guard', () => {
  it('sends a bare history address to its first table', () => {
    expect(() => guardHistorySection(undefined, '/i/acme/history')).toThrow();
  });

  it('leaves a named section alone', () => {
    expect(() => guardHistorySection('audit', '/i/acme/history/audit')).not.toThrow();
    expect(() => guardHistorySection('failures', '/i/acme/history/failures')).not.toThrow();
  });
});

describe('root access route guard', () => {
  it('accepts known user and invitation dialogs', () => {
    expect(() => guardRootAccessRest('users', 'add')).not.toThrow();
    expect(() => guardRootAccessRest('users', 'octocat/ban')).not.toThrow();
    expect(() => guardRootAccessRest('invitations', 'new')).not.toThrow();
    expect(() => guardRootAccessRest('invitations', 'invite-1/reissue')).not.toThrow();
  });

  it('rejects unknown or overlong dialog paths', () => {
    expect(() => guardRootAccessRest('users', 'octocat/history')).toThrow();
    expect(() => guardRootAccessRest('invitations', 'invite-1/reissue/extra')).toThrow();
  });
});

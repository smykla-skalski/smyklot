import { describe, expect, it } from 'vitest';

import { guardPanelViewRest, guardRootAccessRest } from '../src/lib/route-guard.ts';

describe('panel view route guard', () => {
  it('accepts base views and known path dialogs', () => {
    expect(() => guardPanelViewRest('settings', undefined, '/i/acme/settings')).not.toThrow();
    expect(() =>
      guardPanelViewRest(
        'repositories',
        'api-gateway/commands',
        '/i/acme/repositories/api-gateway/commands',
      ),
    ).not.toThrow();
    expect(() =>
      guardPanelViewRest('users', 'octocat/history', '/i/acme/users/octocat/history'),
    ).not.toThrow();
  });

  it('rejects trailing segments a view cannot render', () => {
    expect(() => guardPanelViewRest('settings', 'extra', '/i/acme/settings/extra')).toThrow();
    expect(() =>
      guardPanelViewRest(
        'repositories',
        'api-gateway/unknown',
        '/i/acme/repositories/api-gateway/unknown',
      ),
    ).toThrow();
    expect(() => guardPanelViewRest('history', 'unknown', '/i/acme/history/unknown')).toThrow();
  });

  it('redirects bare history to its canonical audit address', () => {
    expect(() => guardPanelViewRest('history', undefined, '/i/acme/history')).toThrow();
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

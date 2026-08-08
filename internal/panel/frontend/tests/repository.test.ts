import { describe, expect, it } from 'vitest';

import { PanelApiError } from '../src/lib/api';
import {
  shouldClearFailureAfterAutomaticRefresh,
  shouldReloadRepositoryAfterSaveFailure,
  shouldReplaceFailureWithReadError,
} from '../src/lib/repository';

describe('repository save recovery', () => {
  it('reloads stale state only after a revision conflict', () => {
    expect(
      shouldReloadRepositoryAfterSaveFailure(
        new PanelApiError(409, 'revision_conflict', 'settings changed elsewhere'),
      ),
    ).toBe(true);
    expect(
      shouldReloadRepositoryAfterSaveFailure(
        new PanelApiError(400, 'invalid_request', 'settings are invalid'),
      ),
    ).toBe(false);
    expect(shouldReloadRepositoryAfterSaveFailure(new TypeError('network failed'))).toBe(false);
  });

  it('preserves write failures while automatic refreshes reconcile repository data', () => {
    expect(shouldClearFailureAfterAutomaticRefresh('read')).toBe(true);
    expect(shouldClearFailureAfterAutomaticRefresh('write')).toBe(false);
    expect(shouldReplaceFailureWithReadError(undefined)).toBe(true);
    expect(shouldReplaceFailureWithReadError('read')).toBe(true);
    expect(shouldReplaceFailureWithReadError('write')).toBe(false);
  });
});

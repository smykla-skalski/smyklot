import { describe, expect, it } from 'vitest';

import { fixtureApi } from '../stories/support/api';
import { SYNC_FILES_CONTEXT, SYNC_STATUS, TARGET } from '../stories/support/fixtures';

describe('catalogue fixtures [Unit]', () => {
  it('answers every independent read needed to open the sync view', async () => {
    const api = fixtureApi();
    const [status, context] = await Promise.all([
      api.fetchSyncStatus(TARGET.id),
      api.fetchSyncFilesContext(TARGET.id),
    ]);

    expect(status).toEqual(SYNC_STATUS);
    expect(context).toEqual(SYNC_FILES_CONTEXT);
    expect(context.repositories).toBe(status.repositories.length);
    expect(context.covered).toBe(
      status.repositories.filter((row) => row.cells.files.state !== 'off').length,
    );
  });
});

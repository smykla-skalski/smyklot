import { describe, expect, it } from 'vitest';
import { PanelApiError } from '../src/lib/api';
import { syncResultProblem } from '../src/lib/sync-result-error';

describe('sync result read recovery', () => {
  it.each([401, 403, 404])('does not offer a futile retry for %i', (status) => {
    const result = syncResultProblem(new PanelApiError(status, 'unavailable', 'Internal detail'));
    expect(result.retry).toBe(false);
    expect(result.description).not.toContain('Internal detail');
  });
  it.each([new Error('Disconnected'), new PanelApiError(503, 'unavailable', 'Provider failure')])(
    'offers a read retry without promising execution',
    (error) => {
      const result = syncResultProblem(error);
      expect(result.retry).toBe(true);
      expect(result.description).toContain('does not run sync');
    },
  );
});

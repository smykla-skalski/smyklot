import { expect, it } from 'vitest';
import { queueProfileName, queueRepositoryName } from '../src/lib/queue-names';

it('preserves owner context and supports compact names', () => {
  const item = { repository_id: '4002', repository_name: 'owner/project' };
  expect(queueRepositoryName(item)).toBe('owner/project');
  expect(queueRepositoryName(item, true)).toBe('project');
});
it('retains an identifiable fallback when a repository name is unavailable', () => {
  expect(queueRepositoryName({ repository_id: '4002', repository_name: ' ' })).toBe(
    'Repository 4002 (name unavailable)',
  );
  expect(queueRepositoryName({})).toBeNull();
});
it('names hours without exposing the profile slug as its ordinary label', () => {
  expect(queueProfileName('always-open', 'Always open')).toBe('Always open');
  expect(queueProfileName('immediate')).toBe('Immediate');
  expect(queueProfileName('deleted')).toBe('Hours profile deleted (name unavailable)');
});

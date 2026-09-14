import { expect, it } from 'vitest';
import { createPanelApi } from '../src/lib/api';

it.each([
  null,
  'https://github.com/apps/deployed/installations/new',
  'https://git.example/github-apps/deployed/installations/new',
])('accepts installation result %s', async (installation_url) => {
  const api = createPanelApi('', async () => new Response(JSON.stringify({ installation_url })));
  expect(await api.fetchInstallation()).toBe(installation_url);
});
it.each([
  {},
  { installation_url: 3 },
  { installation_url: 'javascript:alert(1)' },
  { installation_url: 'https://github.com/apps/bot' },
  { installation_url: 'https://u:p@github.com/apps/bot/installations/new' },
])('rejects malformed installation result %j', async (body) => {
  const api = createPanelApi('', async () => new Response(JSON.stringify(body)));
  await expect(api.fetchInstallation()).rejects.toThrow();
});

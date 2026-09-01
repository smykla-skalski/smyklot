import { describe, expect, it } from 'vitest';

import {
  readLastConsolePage,
  readLastWorkspacePage,
  writeLastPage,
  type PanelSide,
} from '../src/lib/last-page.ts';

function memoryStorage(): Storage {
  const entries = new Map<string, string>();

  return {
    get length(): number {
      return entries.size;
    },
    clear: (): void => void entries.clear(),
    getItem: (key: string): string | null => entries.get(key) ?? null,
    key: (index: number): string | null => [...entries.keys()][index] ?? null,
    removeItem: (key: string): void => void entries.delete(key),
    setItem: (key: string, value: string): void => void entries.set(key, String(value)),
  };
}

/** One side's key, written the way a tab that had been meddled with would hold it. */
function storedWith(side: PanelSide, value: string): Storage {
  const storage = memoryStorage();
  storage.setItem(`smyklot.panel.last-page.${side}`, value);

  return storage;
}

describe('the page each side was left on [Unit]', () => {
  it('reads back the workspace page it was given, repository and all', () => {
    const storage = memoryStorage();
    writeLastPage(
      'workspace',
      '/workspace/[account]/repositories/[repository]',
      { account: 'acme', repository: 'api-gateway' },
      storage,
    );

    expect(readLastWorkspacePage(storage)).toEqual({
      account: 'acme',
      view: 'repositories',
      repository: { name: 'api-gateway' },
    });
  });

  it('reads back the console page it was given', () => {
    const storage = memoryStorage();
    writeLastPage('console', '/root/queue/request/[id]', { id: 'req-7' }, storage);

    expect(readLastConsolePage(storage)).toEqual({ rootView: 'queue-request', request: 'req-7' });
  });

  it('keeps the two sides apart', () => {
    const storage = memoryStorage();
    writeLastPage('console', '/root/workspaces', {}, storage);
    writeLastPage(
      'workspace',
      '/workspace/[account]/[view=panelView]',
      { account: 'acme', view: 'settings' },
      storage,
    );

    expect(readLastConsolePage(storage)).toEqual({ rootView: 'workspaces' });
    expect(readLastWorkspacePage(storage)).toEqual({ account: 'acme', view: 'settings' });
  });

  it('drops pages remembered under removed route vocabulary', () => {
    const storage = memoryStorage();
    writeLastPage(
      'workspace',
      '/workspace/[account]/[view=panelView]',
      { account: 'acme', view: 'defaults' },
      storage,
    );

    expect(readLastWorkspacePage(storage)).toBeNull();

    writeLastPage('console', '/root/runtime', {}, storage);
    expect(readLastConsolePage(storage)).toEqual({ rootView: 'runtime-service' });
  });

  it('reads back each addressable Runtime leaf', () => {
    const storage = memoryStorage();
    writeLastPage('console', '/root/runtime/service', {}, storage);
    expect(readLastConsolePage(storage)).toEqual({ rootView: 'runtime-service' });

    /* A page remembered before the database joined Service health reads back as
       the page it now lands on, rather than as a leaf that no longer exists. */
    writeLastPage('console', '/root/runtime/database', {}, storage);
    expect(readLastConsolePage(storage)).toEqual({ rootView: 'runtime-service' });

    writeLastPage('console', '/root/runtime/settings', {}, storage);
    expect(readLastConsolePage(storage)).toEqual({ rootView: 'runtime-settings' });
  });

  it('reads a page stored under the other side as nothing remembered', () => {
    const storage = memoryStorage();
    writeLastPage(
      'console',
      '/workspace/[account]/[view=panelView]',
      { account: 'acme', view: 'settings' },
      storage,
    );

    expect(readLastConsolePage(storage)).toBeNull();
  });

  it.each([
    ['nothing stored', memoryStorage()],
    ['a value that is not JSON', storedWith('workspace', '/workspace/acme/settings')],
    ['a value that is not an object', storedWith('workspace', '"/workspace/acme/settings"')],
    [
      'a route id this build does not have',
      storedWith('workspace', '{"id":"/workspace/[account]/billing","params":{"account":"acme"}}'),
    ],
    [
      'parameters that are not a record of strings',
      storedWith(
        'workspace',
        '{"id":"/workspace/[account]/[view=panelView]","params":{"account":7}}',
      ),
    ],
    [
      'no parameters at all',
      storedWith('workspace', '{"id":"/workspace/[account]/[view=panelView]"}'),
    ],
  ])('reads %s as nothing remembered', (_case, storage) => {
    expect(readLastWorkspacePage(storage)).toBeNull();
  });

  it('reads nothing and writes nothing when the tab has no storage', () => {
    expect(readLastWorkspacePage(null)).toBeNull();
    expect(readLastConsolePage(null)).toBeNull();
    expect(() => writeLastPage('console', '/root/workspaces', {}, null)).not.toThrow();
  });

  it('costs nothing but the memory when the store refuses the write', () => {
    const storage = memoryStorage();
    storage.setItem = (): never => {
      throw new DOMException('QuotaExceededError');
    };

    expect(() => writeLastPage('console', '/root/workspaces', {}, storage)).not.toThrow();
  });
});

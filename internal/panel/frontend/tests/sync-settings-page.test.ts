// @vitest-environment jsdom
import { readFileSync } from 'node:fs';

import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import SyncSettingsPage, { SETTING_REQUIRES } from '../src/lib/components/SyncSettingsPage.svelte';
import { chooseOption } from './support/select';
import type { SyncConfig } from '../src/lib/types';

/** The segmented control measures itself to place its thumb; jsdom does not. */
class TestResizeObserver {
  observe(): void {}
  disconnect(): void {}
}

/**
 * The settings page's whole job is the difference between "off" and "nobody
 * said". Only managed settings render as rows; everything unmanaged is one
 * sentence per group, and the x removes the management rather than writing
 * a default - against an endpoint that replaces what it is sent, that is
 * somebody else's repository being reset.
 */
describe('SyncSettingsPage [Component]', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', TestResizeObserver);
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  afterEach(() => vi.unstubAllGlobals());

  function config(over: Partial<SyncConfig> = {}): SyncConfig {
    return {
      kind: 'settings',
      enabled: true,
      labels: [],
      allow_removal: false,
      excludes: [],
      revision: 1,
      updated_by: 'bart',
      updated_at: new Date(0).toISOString(),
      digest: '',
      document: { allow_squash_merge: true, has_wiki: false },
      unreadable: false,
      unavailable: '',
      ...over,
    };
  }

  const base = {
    readOnly: false,
    problem: null,
    nowMs: Date.UTC(2026, 7, 18, 12, 0, 0),
    onToggleEnabled: () => {},
    onChangeDocument: () => {},
  };

  /** One managed setting's row, by the words beside it. */
  function row(label: string): HTMLElement {
    const found = [...document.querySelectorAll<HTMLElement>('.policy-row')].find(
      (candidate) => candidate.querySelector('.setting-name')?.textContent?.trim() === label,
    );
    expect(found, `no row for ${label}`).toBeTruthy();
    return found as HTMLElement;
  }

  it('renders only the managed settings as rows, the rest as one sentence', () => {
    render(SyncSettingsPage, { ...base, config: config() });

    expect(document.querySelectorAll('[data-option]')).toHaveLength(2);
    expect(
      row('Squash merging').querySelector<HTMLInputElement>('input[type=checkbox]')!.checked,
    ).toBe(true);
    expect(row('Wiki').querySelector<HTMLInputElement>('input[type=checkbox]')!.checked).toBe(
      false,
    );
    const rests = [...document.querySelectorAll('.rest-say')].map((el) => el.textContent ?? '');
    expect(rests.some((text) => text.includes('Merge commits'))).toBe(true);
  });

  it('sends the flipped value and nothing about the settings nobody touched', async () => {
    const sent: Array<Record<string, unknown>> = [];
    render(SyncSettingsPage, {
      ...base,
      config: config(),
      onChangeDocument: (document: Record<string, unknown>) => {
        sent.push(document);
      },
    });

    const toggle = row('Wiki').querySelector<HTMLInputElement>('input[type="checkbox"]');
    await fireEvent.click(toggle as HTMLInputElement);

    expect(sent).toHaveLength(1);
    expect(sent[0]).toEqual({ allow_squash_merge: true, has_wiki: true });
  });

  it('drops a setting back to following the repository', async () => {
    const sent: Array<Record<string, unknown>> = [];
    render(SyncSettingsPage, {
      ...base,
      config: config(),
      onChangeDocument: (document: Record<string, unknown>) => {
        sent.push(document);
      },
    });

    const clear = screen.getByRole('button', { name: 'Stop managing Wiki' });
    await fireEvent.click(clear as HTMLButtonElement);

    // Removed, not written false: absence is the third state.
    expect(sent).toHaveLength(1);
    expect(sent[0]).toEqual({ allow_squash_merge: true });
  });

  it('manages a picked setting holding a value rather than a hole', async () => {
    const sent: Array<Record<string, unknown>> = [];
    render(SyncSettingsPage, {
      ...base,
      config: config(),
      onChangeDocument: (document: Record<string, unknown>) => {
        sent.push(document);
      },
    });

    await chooseOption(
      screen.getByRole('combobox', { name: 'Manage an option in Merging' }),
      'Merge commits',
    );

    expect(sent).toHaveLength(1);
    expect(sent[0]).toEqual({
      allow_squash_merge: true,
      has_wiki: false,
      allow_merge_commit: true,
    });
  });

  /** What one flip of a managed switch sends, from a document of its own. */
  async function flip(document: Record<string, unknown>, label: string) {
    const sent: Array<Record<string, unknown>> = [];
    render(SyncSettingsPage, {
      ...base,
      config: config({ document }),
      onChangeDocument: (next: Record<string, unknown>) => {
        sent.push(next);
      },
    });
    await fireEvent.click(row(label).querySelector<HTMLInputElement>('input[type="checkbox"]')!);
    expect(sent).toHaveLength(1);
    return sent[0];
  }

  // The server refuses push protection beside secret scanning switched off, so
  // leaving it on made secret scanning impossible to switch off.
  it('switches off what needs a setting along with it', async () => {
    expect(
      await flip(
        { secret_scanning: true, secret_scanning_push_protection: true, advanced_security: false },
        'Secret scanning',
      ),
    ).toEqual({
      secret_scanning: false,
      secret_scanning_push_protection: false,
      advanced_security: false,
    });
  });

  it('follows the chain through a setting nobody manages', async () => {
    expect(
      await flip(
        { advanced_security: true, secret_scanning_push_protection: true, has_wiki: true },
        'Advanced security',
      ),
    ).toEqual({
      advanced_security: false,
      secret_scanning_push_protection: false,
      has_wiki: true,
    });
  });

  it('stops managing a wording whose merge method goes off, since a wording has no off', async () => {
    expect(
      await flip(
        {
          allow_squash_merge: true,
          squash_merge_commit_title: 'PR_TITLE',
          merge_commit_title: 'PR_TITLE',
        },
        'Squash merging',
      ),
    ).toEqual({ allow_squash_merge: false, merge_commit_title: 'PR_TITLE' });
  });

  it('switches nothing else on when a setting goes on', async () => {
    expect(
      await flip(
        { secret_scanning: false, secret_scanning_push_protection: false },
        'Push protection',
      ),
    ).toEqual({ secret_scanning: false, secret_scanning_push_protection: true });
  });

  it('manages a switch off when what it needs is managed off', async () => {
    const sent: Array<Record<string, unknown>> = [];
    render(SyncSettingsPage, {
      ...base,
      config: config({ document: { secret_scanning: false } }),
      onChangeDocument: (document: Record<string, unknown>) => {
        sent.push(document);
      },
    });

    await chooseOption(
      screen.getByRole('combobox', { name: 'Manage an option in Security' }),
      'Push protection',
    );

    expect(sent).toEqual([{ secret_scanning: false, secret_scanning_push_protection: false }]);
  });

  it('knows the same dependencies the server enforces', () => {
    const source = readFileSync('../../orgsync/settings.go', 'utf8');
    const server = [...source.matchAll(/(?:textField|securityField)\("([a-z_]+)", "([a-z_]+)",/gu)]
      .map((match) => [match[1], match[2]])
      .sort();
    expect(server.length).toBeGreaterThan(0);
    expect([...SETTING_REQUIRES].sort()).toEqual(server);
  });

  it('carries the switch that says whether any of this is enforced', async () => {
    const sent: boolean[] = [];
    render(SyncSettingsPage, {
      ...base,
      config: config({ enabled: false }),
      onToggleEnabled: (enabled: boolean) => {
        sent.push(enabled);
      },
    });

    await fireEvent.click(screen.getByLabelText('Enable repository option sync'));

    expect(sent).toEqual([true]);
  });

  it('changes nothing when the stored document could not be read', () => {
    render(SyncSettingsPage, {
      ...base,
      config: config({ document: {}, unreadable: true }),
    });

    expect(screen.getByRole('alert').textContent).toContain('cannot read');
    const controls = document.querySelectorAll(
      'input[type="checkbox"], .icon-button, [role="combobox"]',
    );
    for (const control of controls) {
      expect((control as HTMLInputElement | HTMLButtonElement).disabled).toBe(true);
    }
  });

  it('says which permission is missing while the switch is on', () => {
    const why = 'Smyklot has not been granted administration access, which settings sync needs';
    render(SyncSettingsPage, {
      ...base,
      config: config({ unavailable: why }),
    });

    expect(screen.getByRole('status').textContent).toContain('administration');
  });

  it('says nothing of a permission while the switch is off', () => {
    const why = 'Smyklot has not been granted administration access, which settings sync needs';
    render(SyncSettingsPage, {
      ...base,
      config: config({ enabled: false, unavailable: why }),
    });

    expect(screen.queryByRole('status')).toBeNull();
  });

  it('turns the unmanaged names into rows under Everything', async () => {
    render(SyncSettingsPage, { ...base, config: config() });

    const everything = [...document.querySelectorAll<HTMLInputElement>('input[type="radio"]')].find(
      (held) => held.value === 'everything',
    );
    await fireEvent.click(everything as HTMLInputElement);

    expect(document.querySelectorAll('[data-option]')).toHaveLength(17);
    expect(row('Merge commits').textContent).toContain('From each repository');
  });
});

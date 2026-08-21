// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import SyncSettingsPage from '../src/lib/components/SyncSettingsPage.svelte';
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
    saving: false,
    sectionHref: () => '#',
    onOpenSection: () => {},
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
    render(SyncSettingsPage, { ...base, config: config(), onSave: () => {} });

    expect(document.querySelectorAll('.policy-row')).toHaveLength(2);
    expect(row('Squash merging').textContent).toContain('On');
    expect(row('Wiki').textContent).toContain('Off');
    const rests = [...document.querySelectorAll('.rest-say')].map((el) => el.textContent ?? '');
    expect(rests.some((text) => text.includes('Merge commits'))).toBe(true);
  });

  it('sends the flipped value and nothing about the settings nobody touched', async () => {
    const sent: Array<Record<string, unknown>> = [];
    render(SyncSettingsPage, {
      ...base,
      config: config(),
      onSave: (_enabled: boolean, document: Record<string, unknown>) => {
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
      onSave: (_enabled: boolean, document: Record<string, unknown>) => {
        sent.push(document);
      },
    });

    const clear = row('Wiki').querySelector<HTMLButtonElement>('.setting-clear');
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
      onSave: (_enabled: boolean, document: Record<string, unknown>) => {
        sent.push(document);
      },
    });

    const restButtons = screen.getAllByRole('button', { name: /Manage one/ });
    await fireEvent.click(restButtons[0] as HTMLElement);
    const chip = [...document.querySelectorAll<HTMLButtonElement>('.add-chip')].find((held) =>
      (held.textContent ?? '').includes('Merge commits'),
    );
    await fireEvent.click(chip as HTMLButtonElement);

    expect(sent).toHaveLength(1);
    expect(sent[0]).toEqual({
      allow_squash_merge: true,
      has_wiki: false,
      allow_merge_commit: true,
    });
  });

  it('carries the switch that says whether any of this is enforced', async () => {
    const sent: boolean[] = [];
    render(SyncSettingsPage, {
      ...base,
      config: config({ enabled: false }),
      onSave: (enabled: boolean) => {
        sent.push(enabled);
      },
    });

    await fireEvent.click(screen.getByLabelText('Settings sync'));

    expect(sent).toEqual([true]);
  });

  it('changes nothing when the stored document could not be read', () => {
    render(SyncSettingsPage, {
      ...base,
      config: config({ document: {}, unreadable: true }),
      onSave: () => {},
    });

    expect(screen.getByRole('alert').textContent).toContain('cannot read');
    const controls = document.querySelectorAll(
      'input[type="checkbox"], .setting-clear, button.add-chip',
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
      onSave: () => {},
    });

    expect(screen.getByRole('status').textContent).toContain('administration');
  });

  it('says nothing of a permission while the switch is off', () => {
    const why = 'Smyklot has not been granted administration access, which settings sync needs';
    render(SyncSettingsPage, {
      ...base,
      config: config({ enabled: false, unavailable: why }),
      onSave: () => {},
    });

    expect(screen.queryByRole('status')).toBeNull();
  });

  it('turns the unmanaged names into rows under Everything', async () => {
    render(SyncSettingsPage, { ...base, config: config(), onSave: () => {} });

    const everything = [...document.querySelectorAll<HTMLInputElement>('input[type="radio"]')].find(
      (held) => held.value === 'everything',
    );
    await fireEvent.click(everything as HTMLInputElement);

    expect(document.querySelectorAll('.policy-row')).toHaveLength(17);
    expect(row('Merge commits').textContent).toContain('Follows each repository');
  });
});

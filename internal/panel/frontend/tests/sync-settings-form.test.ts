// @vitest-environment jsdom
import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import SyncSettingsForm from '../src/lib/components/SyncSettingsForm.svelte';

/** The segmented control measures itself to place its thumb; jsdom does not. */
class TestResizeObserver {
  observe(): void {}
  disconnect(): void {}
}

/**
 * The settings page's whole job is the difference between "off" and "nobody
 * said". A page that cannot express the third state turns a policy about two
 * settings into a policy about seventeen, and against an endpoint that replaces
 * what it is sent, that is somebody else's repository being reset.
 *
 * The second job is that the page IS the policy: what this installation decides
 * is a row, and what it leaves alone is one sentence per group.
 */
describe('SyncSettingsForm [Component]', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', TestResizeObserver);
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  afterEach(() => vi.unstubAllGlobals());

  const base = {
    stored: {},
    enabled: false,
    unreadable: false,
    readOnly: false,
    saving: false,
    onSave: () => {},
  };

  /** One setting's row, by the words beside it. */
  function row(container: HTMLElement, label: string): HTMLElement {
    const found = [...container.querySelectorAll<HTMLElement>('.policy-row')].find(
      (candidate) => candidate.querySelector('.setting-name')?.textContent?.trim() === label,
    );
    expect(found, `no row for ${label}`).toBeTruthy();

    return found as HTMLElement;
  }

  function rowNames(container: HTMLElement): string[] {
    return [...container.querySelectorAll<HTMLElement>('.policy-row .setting-name')].map(
      (name) => name.textContent?.trim() ?? '',
    );
  }

  /** The card for one group, by its heading. */
  function group(container: HTMLElement, name: string): HTMLElement {
    const found = [...container.querySelectorAll<HTMLElement>('.plate')].find((candidate) =>
      candidate.textContent?.includes(name),
    );
    expect(found, `no group card for ${name}`).toBeTruthy();

    return found as HTMLElement;
  }

  /**
   * Only what this installation decides is a row. Seventeen switches make a
   * reader work out which nine of them are the policy, which is the only thing
   * on the page worth knowing.
   */
  it('draws a row for what is managed and a sentence for the rest', () => {
    const { container } = render(SyncSettingsForm, {
      ...base,
      stored: { has_wiki: false, has_issues: true },
    });

    // The vocabulary's order, never the document's: a row that moved when
    // somebody managed a different setting is a row nobody can point at.
    expect(rowNames(container)).toEqual(['Issues', 'Wiki']);

    const features = group(container, 'Features');
    expect(features.querySelector('.group-tally')?.textContent).toBe('2 of 4 managed');
    expect(features.querySelector('.group-rest')?.textContent).toContain('Projects');
    expect(features.querySelector('.group-rest')?.textContent).toContain('Discussions');
  });

  /** The value twice: as a word a column is read down, and as the control. */
  it('says a managed value as a word beside its control', () => {
    const { container } = render(SyncSettingsForm, {
      ...base,
      stored: { has_wiki: false, has_issues: true },
    });

    expect(row(container, 'Wiki').querySelector('.value-word')?.textContent).toBe('Off');
    expect(row(container, 'Issues').querySelector('.value-word')?.textContent).toBe('On');
  });

  /**
   * Every control here lands at once. The document is the panel's own record of
   * what should be true and nothing reaches GitHub until a plan is approved, so
   * there is nothing for a Save button to hold back.
   */
  it('writes a change as it is made, keeping the keys it has no control for', async () => {
    const onSave = vi.fn();
    const { container } = render(SyncSettingsForm, {
      ...base,
      enabled: true,
      stored: { has_wiki: false, some_future_key: 'kept' },
      onSave,
    });

    await fireEvent.click(within(row(container, 'Wiki')).getByRole('switch'));

    expect(onSave).toHaveBeenCalledTimes(1);
    expect(onSave.mock.calls[0]?.[0]).toBe(true);
    expect(onSave.mock.calls[0]?.[1]).toEqual({ has_wiki: true, some_future_key: 'kept' });
  });

  /**
   * The opposite of "managed and off" is "not managed", so the clear removes
   * the key rather than writing false: the repository goes back to whatever it
   * says, which is a different sentence from "off everywhere".
   */
  it('drops a setting back to following the repository', async () => {
    const onSave = vi.fn();
    const { container } = render(SyncSettingsForm, {
      ...base,
      stored: { has_wiki: false, has_issues: true },
      onSave,
    });

    await fireEvent.click(within(row(container, 'Wiki')).getByRole('button'));

    expect(onSave.mock.calls[0]?.[1]).toEqual({ has_issues: true });
  });

  /** Managing one starts it where GitHub itself starts a repository. */
  it('starts managing a setting from the picker', async () => {
    const onSave = vi.fn();
    const { container } = render(SyncSettingsForm, {
      ...base,
      stored: { has_wiki: false },
      onSave,
    });

    await fireEvent.click(
      within(group(container, 'Features')).getByRole('button', { name: 'Manage' }),
    );
    await fireEvent.click(screen.getByRole('button', { name: 'Discussions' }));

    expect(onSave.mock.calls[0]?.[1]).toEqual({ has_wiki: false, has_discussions: true });
  });

  /** An instant filter over what is on screen: it saves nothing and asks nothing. */
  it('shows every setting when the filter says everything', async () => {
    const { container } = render(SyncSettingsForm, { ...base, stored: { has_wiki: false } });

    expect(rowNames(container)).toEqual(['Wiki']);

    await fireEvent.click(screen.getByRole('radio', { name: /Everything/ }));

    expect(rowNames(container)).toContain('Issues');
    expect(rowNames(container)).toContain('Squash commit title');
  });

  /** Searching reaches the unmanaged too, because finding one is how it comes to be managed. */
  it('finds a setting nobody manages yet', async () => {
    const { container } = render(SyncSettingsForm, { ...base, stored: { has_wiki: false } });

    await fireEvent.input(screen.getByRole('searchbox'), { target: { value: 'discussion' } });

    expect(rowNames(container)).toEqual(['Discussions']);
  });

  /** The kind's own switch is instant too: it makes the kind eligible for planning. */
  it('carries the switch that says whether any of this is enforced', async () => {
    const onSave = vi.fn();
    render(SyncSettingsForm, { ...base, stored: { has_wiki: false }, onSave });

    await fireEvent.click(screen.getByRole('switch', { name: 'Syncing' }));

    expect(onSave.mock.calls[0]?.[0]).toBe(true);
    expect(onSave.mock.calls[0]?.[1]).toEqual({ has_wiki: false });
  });

  /**
   * The forms on this page save separately and neither waits for the other, so
   * a failure has to be shown beside the one it belongs to. One shared message
   * was one form's failure wiped by the other's next click.
   */
  it('shows a failure of its own beside its own controls', () => {
    render(SyncSettingsForm, { ...base, problem: 'the settings changed; reload' });

    expect(screen.getByRole('alert').textContent).toContain('reload');
  });

  /**
   * A document this version cannot read shows nothing managed, which is what an
   * unconfigured installation looks like too. Writing from that page would send
   * the emptiness back and wipe settings nobody was ever shown.
   */
  it('changes nothing when the stored document could not be read', () => {
    const { container } = render(SyncSettingsForm, {
      ...base,
      stored: { has_wiki: false },
      unreadable: true,
    });

    expect(screen.getByRole('alert').textContent).toContain('cannot read');
    expect(within(row(container, 'Wiki')).getByRole('switch').hasAttribute('disabled')).toBe(true);
    expect(screen.getByRole('switch', { name: 'Syncing' }).hasAttribute('disabled')).toBe(true);
  });

  /**
   * Settings sync is the first kind needing a permission no existing
   * installation has granted, so this is the ordinary first-use answer. Without
   * it the switch goes on, the save succeeds, and the plan list says the same
   * thing it says while waiting for a sweep - forever.
   */
  it('says which permission is missing while the switch is on', () => {
    render(SyncSettingsForm, {
      ...base,
      enabled: true,
      unavailable: 'Smyklot has not been granted administration access, which settings sync needs',
    });

    expect(screen.getByRole('status').textContent).toContain('administration');
  });

  /** Nobody asked for this kind, so nothing is waiting on the permission. */
  it('says nothing of a permission while the switch is off', () => {
    render(SyncSettingsForm, {
      ...base,
      enabled: false,
      unavailable: 'Smyklot has not been granted administration access, which settings sync needs',
    });

    expect(screen.queryByRole('status')).toBeNull();
  });

  /** Reading is offered to everybody; changing is not. */
  it('offers no way to stop managing while read only', () => {
    const { container } = render(SyncSettingsForm, {
      ...base,
      stored: { has_wiki: false },
      readOnly: true,
    });

    expect(within(row(container, 'Wiki')).queryByRole('button')).toBeNull();
  });
});

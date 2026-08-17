// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import SyncSettingsForm from '../src/lib/components/SyncSettingsForm.svelte';

/** The segmented control measures itself to place its thumb; jsdom does not. */
class TestResizeObserver {
  observe(): void {}
  disconnect(): void {}
}

/**
 * The settings form's whole job is the difference between "off" and "nobody
 * said". A control that cannot express the third state turns a policy about
 * two settings into a policy about seventeen, and against an endpoint that
 * replaces what it is sent, that is somebody else's repository being reset.
 */
describe('SyncSettingsForm [Component]', () => {
  // The linked control's tooltip renders into the app shell, which is the page
  // this form is part of everywhere but here.
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
    const found = [...container.querySelectorAll<HTMLElement>('.settings-row')].find(
      (candidate) => candidate.querySelector('.settings-label')?.textContent?.trim() === label,
    );
    expect(found, `no row for ${label}`).toBeTruthy();

    return found as HTMLElement;
  }

  /**
   * The radio a segmented control renders for one value. Matched on the
   * property rather than an attribute selector: Svelte assigns an input's value
   * rather than writing it into the markup.
   */
  function radio(container: HTMLElement, label: string, value: string): HTMLInputElement {
    const input = [
      ...row(container, label).querySelectorAll<HTMLInputElement>('input[type="radio"]'),
    ].find((candidate) => candidate.value === value);
    expect(input, `no ${value} control for ${label}`).toBeTruthy();

    return input as HTMLInputElement;
  }

  it('shows a stored setting as chosen and an absent one as following', () => {
    const { container } = render(SyncSettingsForm, {
      ...base,
      stored: { has_wiki: false, squash_merge_commit_title: 'PR_TITLE' },
    });

    expect(radio(container, 'Wiki', 'off').checked).toBe(true);
    expect(radio(container, 'Wiki', 'on').checked).toBe(false);
    expect(radio(container, 'Squash commit title', 'PR_TITLE').checked).toBe(true);

    // Nobody configured this one, so nothing is chosen - which is not the same
    // as choosing off, and is what leaves the repository alone.
    expect(radio(container, 'Issues', 'on').checked).toBe(false);
    expect(radio(container, 'Issues', 'off').checked).toBe(false);
  });

  it('sends what was chosen, and nothing about the settings nobody touched', async () => {
    const onSave = vi.fn();
    const { container } = render(SyncSettingsForm, { ...base, onSave });

    await fireEvent.click(radio(container, 'Wiki', 'off'));
    await fireEvent.click(radio(container, 'Merge commit title', 'PR_TITLE'));
    await fireEvent.click(screen.getByRole('button', { name: 'Save settings' }));

    expect(onSave).toHaveBeenCalledTimes(1);
    expect(onSave.mock.calls[0]?.[1]).toEqual({
      has_wiki: false,
      merge_commit_title: 'PR_TITLE',
    });
  });

  it('drops a setting back to following the repository', async () => {
    const onSave = vi.fn();
    const { container } = render(SyncSettingsForm, {
      ...base,
      stored: { has_wiki: false, has_issues: true },
      onSave,
    });

    // The restore button belongs to the setting whose chain is broken, which is
    // every configured one.
    const restore = row(container, 'Wiki').querySelector<HTMLButtonElement>('button.broken');
    expect(restore, 'a configured setting offers no way back').toBeTruthy();

    await fireEvent.click(restore as HTMLButtonElement);
    await fireEvent.click(screen.getByRole('button', { name: 'Save settings' }));

    expect(onSave.mock.calls[0]?.[1]).toEqual({ has_issues: true });
  });

  it('carries the switch that says whether any of this is enforced', async () => {
    const onSave = vi.fn();
    render(SyncSettingsForm, { ...base, stored: { has_wiki: false }, onSave });

    await fireEvent.click(screen.getByRole('checkbox'));
    await fireEvent.click(screen.getByRole('button', { name: 'Save settings' }));

    expect(onSave.mock.calls[0]?.[0]).toBe(true);
  });

  /**
   * The labels form on the same page saves separately and neither waits for the
   * other, so a failure has to be shown beside the form it belongs to. One
   * shared message was one form's failure wiped by the other's next click.
   */
  it('shows a failure of its own beside its own controls', () => {
    render(SyncSettingsForm, { ...base, problem: 'the settings changed; reload' });

    expect(screen.getByRole('alert').textContent).toContain('reload');
  });

  it('offers no save while nothing has changed', () => {
    render(SyncSettingsForm, { ...base, stored: { has_wiki: false } });

    expect(screen.getByRole('button', { name: 'Save settings' }).hasAttribute('disabled')).toBe(
      true,
    );
  });

  /**
   * A document this version cannot read shows nothing chosen, which is what an
   * unconfigured installation looks like too. Saving from that form would send
   * the emptiness back and wipe settings nobody was ever shown.
   */
  it('changes nothing when the stored document could not be read', () => {
    const { container } = render(SyncSettingsForm, { ...base, unreadable: true });

    expect(screen.getByRole('alert').textContent).toContain('cannot read');

    // The whole control is disabled at its fieldset, which is where a
    // segmented control takes it.
    expect(row(container, 'Wiki').querySelector('fieldset')?.disabled).toBe(true);
    expect(screen.getByRole('button', { name: 'Save settings' }).hasAttribute('disabled')).toBe(
      true,
    );
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

  /**
   * And the save is still offered: configuring first and granting after is the
   * ordinary order, and a form that refused would leave somebody nothing to do
   * but wait for a permission they may be the one to ask for.
   */
  it('still saves what it is given while the permission is missing', async () => {
    const onSave = vi.fn();
    render(SyncSettingsForm, {
      ...base,
      unavailable: 'Smyklot has not been granted administration access, which settings sync needs',
      onSave,
    });

    await fireEvent.click(screen.getByRole('checkbox'));

    expect(screen.getByRole('status').textContent).toContain('administration');

    await fireEvent.click(screen.getByRole('button', { name: 'Save settings' }));

    expect(onSave.mock.calls[0]?.[0]).toBe(true);
  });
});

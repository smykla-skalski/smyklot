// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import SyncLabelsForm from '../src/lib/components/SyncLabelsForm.svelte';
import type { SyncLabel } from '../src/lib/types.js';

/** The segmented control measures itself to place a thumb; jsdom does not. */
class TestResizeObserver {
  observe(): void {}
  disconnect(): void {}
}

/**
 * The label set every repository in an installation carries is whatever this
 * form sends. Anything it drops on the way to a save is a label that stops
 * being synchronized, and anything it adds that nobody typed is a change
 * proposed against every repository at once.
 */
describe('SyncLabelsForm [Component]', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', TestResizeObserver);
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  afterEach(() => vi.unstubAllGlobals());

  const base = {
    labels: [] as SyncLabel[],
    allowRemoval: false,
    excludes: [] as string[],
    enabled: false,
    unreadable: false,
    readOnly: false,
    saving: false,
    onSave: () => {},
  };

  function bug(over: Partial<SyncLabel> = {}): SyncLabel {
    return { name: 'kind/bug', color: 'd73a4a', description: "Something isn't working", ...over };
  }

  /**
   * A label the installation has said nothing about the description of, which
   * is a missing key rather than an undefined one.
   *
   * Written out rather than spread with `description: undefined`, which is what
   * this was first: spreading sets the key, so the fixture said "present and
   * undefined" while claiming to say "absent" - and the server cannot send that
   * shape at all, because JSON has no undefined.
   */
  function undescribed(): SyncLabel {
    return { name: 'kind/bug', color: 'd73a4a' };
  }

  function saved() {
    const sent: {
      enabled: boolean;
      labels: SyncLabel[];
      allowRemoval: boolean;
      excludes: string[];
    }[] = [];

    return {
      sent,
      onSave: (enabled: boolean, labels: SyncLabel[], allowRemoval: boolean, excludes: string[]) =>
        sent.push({ enabled, labels, allowRemoval, excludes }),
    };
  }

  async function save(): Promise<void> {
    await fireEvent.click(screen.getByRole('button', { name: 'Save labels' }));
  }

  it('shows the labels the installation configured', () => {
    render(SyncLabelsForm, { ...base, labels: [bug()] });

    expect(screen.getByDisplayValue('kind/bug')).toBeTruthy();
    expect(screen.getByDisplayValue('d73a4a')).toBeTruthy();
    expect(screen.getByDisplayValue("Something isn't working")).toBeTruthy();
  });

  it('says so where nothing is configured rather than showing an empty row', () => {
    render(SyncLabelsForm, base);

    expect(screen.getByText('No labels yet.')).toBeTruthy();
  });

  it('offers no save until something changes', () => {
    render(SyncLabelsForm, base);

    expect(screen.getByRole('button', { name: 'Save labels' }).hasAttribute('disabled')).toBe(true);
  });

  it('sends a label somebody added', async () => {
    const { sent, onSave } = saved();
    render(SyncLabelsForm, { ...base, onSave });

    await fireEvent.click(screen.getByRole('button', { name: 'Add a label' }));
    await fireEvent.change(screen.getByPlaceholderText('kind/bug'), {
      target: { value: 'area/ci' },
    });
    await fireEvent.change(screen.getByPlaceholderText('d73a4a'), { target: { value: 'fbca04' } });
    await save();

    expect(sent[0].labels).toEqual([{ name: 'area/ci', color: 'fbca04' }]);
  });

  it('sends a label somebody removed', async () => {
    const { sent, onSave } = saved();
    render(SyncLabelsForm, { ...base, labels: [bug()], onSave });

    await fireEvent.click(screen.getByRole('button', { name: 'Remove' }));
    await save();

    expect(sent[0].labels).toEqual([]);
  });

  /**
   * A description has three states where the box has two, and this is the one
   * that gives.
   *
   * Absent means "leave whatever each repository wrote"; an empty string asks
   * every repository to have its description cleared. Those read identically on
   * screen, so if an emptied box meant clearing, a description typed and then
   * thought better of would go out as an instruction to wipe that label's
   * description across the organization - and there would be no way back to
   * leaving it alone without reloading the page.
   */
  describe('a description', () => {
    it('is left off entirely where the box is empty', async () => {
      const { sent, onSave } = saved();
      render(SyncLabelsForm, { ...base, labels: [undescribed()], onSave });

      await fireEvent.change(screen.getByPlaceholderText('kind/bug'), {
        target: { value: 'kind/defect' },
      });
      await save();

      expect(sent[0].labels).toHaveLength(1);
      expect('description' in sent[0].labels[0]).toBe(false);
    });

    it('goes back to absent when somebody empties one that was set', async () => {
      const { sent, onSave } = saved();
      render(SyncLabelsForm, { ...base, labels: [bug()], onSave });

      await fireEvent.change(screen.getByDisplayValue("Something isn't working"), {
        target: { value: '' },
      });
      await save();

      expect('description' in sent[0].labels[0]).toBe(false);
    });

    /**
     * The state this form cannot show, arriving from somewhere that could write
     * it - the API takes an empty string and means "clear it everywhere" by it.
     *
     * Rendered, it is an empty box, identical to one that leaves each repository
     * alone. Nothing a person does to an empty box fires a change event, so
     * without healing it on the way in there is no way to tell it is there and
     * no way to take it off, and every save from this form would carry that
     * standing instruction along with whatever else it changed.
     */
    it('takes an empty one off a row that arrived carrying it', async () => {
      const { sent, onSave } = saved();
      render(SyncLabelsForm, { ...base, labels: [bug({ description: '' })], onSave });

      // Somewhere else on the row, so the save is about anything but this field.
      await fireEvent.change(screen.getByPlaceholderText('d73a4a'), {
        target: { value: 'fbca04' },
      });
      await save();

      expect('description' in sent[0].labels[0]).toBe(false);
    });

    /**
     * Healing one on the way in must not read as somebody having edited it.
     *
     * The saved side is compared against the draft, and the draft is the healed
     * one - so unless the saved side is healed the same way, a stored empty
     * description puts Save live the moment the page opens, on a form nobody
     * has touched. The healing still happens: it rides along with the next save
     * made for any other reason.
     */
    it('does not offer a save just because it healed one', () => {
      render(SyncLabelsForm, { ...base, labels: [bug({ description: '' })] });

      expect(screen.getByRole('button', { name: 'Save labels' }).hasAttribute('disabled')).toBe(
        true,
      );
    });

    it('is sent where somebody wrote one', async () => {
      const { sent, onSave } = saved();
      render(SyncLabelsForm, { ...base, labels: [undescribed()], onSave });

      await fireEvent.change(screen.getByPlaceholderText("Something isn't working"), {
        target: { value: 'A defect' },
      });
      await save();

      expect(sent[0].labels[0].description).toBe('A defect');
    });
  });

  /**
   * The only control here that destroys anything. A label this list does not
   * name goes on existing unless this is on, so it is off until somebody says
   * otherwise and the list of names to spare is on the same page as the switch
   * that needs it.
   */
  it('sends removal and the names it spares', async () => {
    const { sent, onSave } = saved();
    render(SyncLabelsForm, { ...base, labels: [bug()], onSave });

    await fireEvent.click(screen.getByRole('radio', { name: 'On' }));
    await fireEvent.change(screen.getByPlaceholderText('hand-made-*'), {
      target: { value: 'hand-made-*\ndependencies' },
    });
    await save();

    expect(sent[0].allowRemoval).toBe(true);
    expect(sent[0].excludes).toEqual(['hand-made-*', 'dependencies']);
  });

  /**
   * A configuration this version cannot read shows an empty list because
   * nothing came out of the stored document, not because nothing is configured.
   * Saving from that would send the emptiness back and wipe a label set nobody
   * was ever shown.
   */
  it('cannot be edited while what is stored cannot be read', () => {
    render(SyncLabelsForm, { ...base, unreadable: true });

    expect(screen.getByRole('button', { name: 'Add a label' }).hasAttribute('disabled')).toBe(true);
  });
});

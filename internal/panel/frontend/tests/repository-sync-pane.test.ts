// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import RepositorySyncPane from '../src/lib/components/RepositorySyncPane.svelte';
import type { SyncOverride } from '../src/lib/types';

/** The controls measure themselves to place a thumb; jsdom does not. */
class TestResizeObserver {
  observe(): void {}
  disconnect(): void {}
}

/**
 * What this pane saves is the only thing that makes a repository's own
 * customization survive a sync. Drop it and the plain template is written over
 * exactly the file it described - the failure this whole port exists to stop.
 */
describe('RepositorySyncPane [Component]', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', TestResizeObserver);
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  afterEach(() => vi.unstubAllGlobals());

  function override(over: Partial<SyncOverride> = {}): SyncOverride {
    return {
      kind: 'files',
      enabled: null,
      document: {},
      revision: 1,
      unreadable: false,
      ...over,
    };
  }

  function saved() {
    const sent: { enabled: boolean | null; document: Record<string, unknown> }[] = [];

    return {
      sent,
      onSave: (enabled: boolean | null, document: Record<string, unknown>) =>
        sent.push({ enabled, document }),
    };
  }

  /** A fixed clock, so a relative time reads the same on every run. */
  const now = Date.parse('2026-08-09T10:00:00Z');

  const base = { readOnly: false, saving: false, now, onSave: () => {} };

  async function save(): Promise<void> {
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  }

  it('says so where a repository takes every file as it is written', () => {
    render(RepositorySyncPane, { ...base, stored: override() });

    expect(screen.getByText(/takes every file as the organization writes it/)).toBeTruthy();
  });

  it('shows what a repository already adjusts', () => {
    render(RepositorySyncPane, {
      ...base,
      stored: override({
        document: {
          merges: [{ path: 'renovate.json', overrides: { timezone: 'Europe/Warsaw' } }],
        },
      }),
    });

    expect(screen.getByDisplayValue('renovate.json')).toBeTruthy();

    const overrides = screen.getByLabelText('What this repository sets') as HTMLTextAreaElement;
    expect(overrides.value).toContain('Europe/Warsaw');
  });

  it('sends an adjustment somebody wrote', async () => {
    const { sent, onSave } = saved();
    render(RepositorySyncPane, { ...base, stored: override(), onSave });

    await fireEvent.click(screen.getByRole('button', { name: 'Adjust a file' }));
    await fireEvent.change(screen.getByLabelText('File'), {
      target: { value: 'renovate.json' },
    });
    await fireEvent.change(screen.getByLabelText('What this repository sets'), {
      target: { value: '{"timezone": "Europe/Warsaw"}' },
    });
    await save();

    expect(sent).toHaveLength(1);
    expect(sent[0].document.merges).toEqual([
      { path: 'renovate.json', overrides: { timezone: 'Europe/Warsaw' } },
    ]);
  });

  /**
   * A half-typed object is not an object. Sending one would either store
   * something nobody wrote or be refused by the server with a message about
   * JSON, so the refusal happens here where the box is.
   */
  it('refuses to save what is not a JSON object', async () => {
    render(RepositorySyncPane, {
      ...base,
      stored: override({ document: { merges: [{ path: 'renovate.json' }] } }),
    });

    await fireEvent.change(screen.getByLabelText('What this repository sets'), {
      target: { value: '{"timezone": ' },
    });

    expect(screen.getByRole('alert').textContent).toContain('not a JSON object');
    expect(screen.getByRole('button', { name: 'Save' }).hasAttribute('disabled')).toBe(true);
  });

  it('sends the files this repository wants left alone', async () => {
    const { sent, onSave } = saved();
    render(RepositorySyncPane, { ...base, stored: override(), onSave });

    await fireEvent.change(screen.getByLabelText('Files to leave alone here'), {
      target: { value: 'renovate.json\nCONTRIBUTING.md' },
    });
    await save();

    expect(sent[0].document.excludes).toEqual(['renovate.json', 'CONTRIBUTING.md']);
  });

  /**
   * Three states rather than two. "Inherits, and the installation says no" and
   * "this repository says no" are different answers that stop being the same
   * the moment the installation changes its mind.
   */
  it('sends nothing about enablement while the repository inherits', async () => {
    const { sent, onSave } = saved();
    render(RepositorySyncPane, { ...base, stored: override(), onSave });

    await fireEvent.change(screen.getByLabelText('Files to leave alone here'), {
      target: { value: 'LICENSE' },
    });
    await save();

    expect(sent[0].enabled).toBeNull();
  });

  it('offers no save until something changes', () => {
    render(RepositorySyncPane, { ...base, stored: override() });

    expect(screen.getByRole('button', { name: 'Save' }).hasAttribute('disabled')).toBe(true);
  });

  /**
   * Two documents that would be saved the same way have to compare the same
   * way, whatever order their keys arrived in. Comparing the raw text put Save
   * live the moment the page loaded, for a document nobody had touched.
   */
  it('offers no save for a document whose keys arrived in another order', () => {
    render(RepositorySyncPane, {
      ...base,
      stored: override({
        document: {
          excludes: ['LICENSE'],
          merges: [{ path: 'renovate.json', overrides: { timezone: 'UTC' } }],
        },
      }),
    });

    expect(screen.getByRole('button', { name: 'Save' }).hasAttribute('disabled')).toBe(true);
  });

  /**
   * Anything a later version adds is stored in the same document, and a form
   * that rebuilt it from its own controls would drop every key it has no
   * control for.
   */
  it('carries through a key it has no control for', async () => {
    const { sent, onSave } = saved();
    render(RepositorySyncPane, {
      ...base,
      stored: override({ document: { something_later: { deep: true } } }),
      onSave,
    });

    await fireEvent.change(screen.getByLabelText('Files to leave alone here'), {
      target: { value: 'LICENSE' },
    });
    await save();

    expect(sent[0].document.something_later).toEqual({ deep: true });
  });

  /**
   * A document this version cannot read renders as a repository adjusting
   * nothing, which is what somebody would then save over.
   */
  it('changes nothing while the stored document cannot be read', () => {
    render(RepositorySyncPane, { ...base, stored: override({ unreadable: true }) });

    expect(screen.getByRole('alert').textContent).toContain('cannot read');
    expect(screen.getByRole('button', { name: 'Save' }).hasAttribute('disabled')).toBe(true);
  });

  it('offers nothing to change to somebody who may only read', () => {
    render(RepositorySyncPane, {
      ...base,
      readOnly: true,
      stored: override({ document: { merges: [{ path: 'renovate.json' }] } }),
    });

    expect(screen.queryByRole('button', { name: 'Save' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Adjust a file' })).toBeNull();
  });

  /**
   * A repository the planner refuses is receiving none of the organization's
   * files. Before this notice it looked here exactly like one receiving all of
   * them, and the only account of why was a line in the service log.
   */
  it('says why this repository is getting none of the files', () => {
    render(RepositorySyncPane, {
      ...base,
      stored: override({
        problem: 'these files cannot be composed: docs is not a directory in this repository',
        problem_at: '2026-08-09T09:57:00Z',
      }),
    });

    const notice = screen.getByRole('status');
    expect(notice.textContent).toContain('are not being synced here');
    expect(notice.textContent).toContain('docs is not a directory in this repository');

    // And when it was found, so a fix saved a minute ago can be told from one
    // this notice already knows about.
    expect(notice.textContent).toContain('3 minutes ago');
  });

  it('says nothing where the planner found nothing wrong', () => {
    render(RepositorySyncPane, { ...base, stored: override() });

    expect(screen.queryByRole('status')).toBeNull();
  });

  /**
   * The three things the merge engine has always implemented and this pane
   * could not say. Nine of the organization's thirteen repositories adjust a
   * template, and six of them do it with a list rule or a heading - so without
   * these the cutover writes the plain template over exactly those six.
   */
  describe('what a repository does beyond setting keys', () => {
    it('appends to a list rather than replacing it', async () => {
      const { sent, onSave } = saved();
      render(RepositorySyncPane, {
        ...base,
        stored: override({ document: { merges: [{ path: 'renovate.json' }] } }),
        onSave,
      });

      await fireEvent.click(screen.getByRole('button', { name: 'Add a list rule' }));
      await fireEvent.change(screen.getByLabelText('List'), {
        target: { value: '$.packageRules' },
      });
      await save();

      expect(sent[0].document.merges).toEqual([
        { path: 'renovate.json', arrays: [{ path: '$.packageRules', strategy: 'append' }] },
      ]);
    });

    /*
     * A list with no rule is replaced whole, so there is nothing left to
     * deduplicate and the engine refuses the flag standing on its own.
     */
    it('offers deduplication only beside a list rule', async () => {
      render(RepositorySyncPane, {
        ...base,
        stored: override({ document: { merges: [{ path: 'renovate.json' }] } }),
      });

      expect(screen.queryByText('Drop repeated entries')).toBeNull();

      await fireEvent.click(screen.getByRole('button', { name: 'Add a list rule' }));

      expect(screen.getByText('Drop repeated entries')).toBeTruthy();
    });

    it('writes deduplication beside the rule it belongs to', async () => {
      const { sent, onSave } = saved();
      render(RepositorySyncPane, {
        ...base,
        stored: override({
          document: {
            merges: [
              { path: 'renovate.json', arrays: [{ path: '$.extends', strategy: 'append' }] },
            ],
          },
        }),
        onSave,
      });

      await fireEvent.click(screen.getByRole('radio', { name: 'On' }));
      await save();

      expect(sent[0].document.merges).toEqual([
        {
          path: 'renovate.json',
          arrays: [{ path: '$.extends', strategy: 'append' }],
          deduplicate: true,
        },
      ]);
    });

    it('never writes deduplication without the rule it belongs to', async () => {
      const { sent, onSave } = saved();
      render(RepositorySyncPane, {
        ...base,
        stored: override({
          document: {
            merges: [
              {
                path: 'renovate.json',
                arrays: [{ path: '$.extends', strategy: 'append' }],
                deduplicate: true,
              },
            ],
          },
        }),
        onSave,
      });

      // The second: the first removes the adjustment, this one the rule inside it.
      await fireEvent.click(screen.getAllByRole('button', { name: 'Remove' })[1]);
      await save();

      expect(sent[0].document.merges).toEqual([{ path: 'renovate.json' }]);
    });

    /*
     * Which controls a row gets follows the engine's own reading: the strategy
     * where it says one, the extension where it does not.
     */
    it('edits a Markdown file by its headings rather than by keys', () => {
      render(RepositorySyncPane, {
        ...base,
        stored: override({ document: { merges: [{ path: 'CONTRIBUTING.md' }] } }),
      });

      expect(screen.queryByLabelText('What this repository sets')).toBeNull();
      expect(screen.getByRole('button', { name: 'Edit a section' })).toBeTruthy();
    });

    it('writes a heading with the marks the document writes it with', async () => {
      const { sent, onSave } = saved();
      render(RepositorySyncPane, {
        ...base,
        stored: override({ document: { merges: [{ path: 'CONTRIBUTING.md' }] } }),
        onSave,
      });

      await fireEvent.click(screen.getByRole('button', { name: 'Edit a section' }));
      await fireEvent.change(screen.getByLabelText('Heading'), {
        target: { value: '### Prerequisites' },
      });
      await fireEvent.change(screen.getByLabelText('What this repository writes'), {
        target: { value: '### Project setup' },
      });
      await save();

      expect(sent[0].document.merges).toEqual([
        {
          path: 'CONTRIBUTING.md',
          sections: [
            { action: 'after', heading: '### Prerequisites', content: '### Project setup' },
          ],
        },
      ]);
    });

    /*
     * Appending addresses the document rather than a heading, and the engine
     * refuses one carrying a heading rather than ignoring it.
     */
    it('drops the heading where a section addresses the whole document', async () => {
      const { sent, onSave } = saved();
      render(RepositorySyncPane, {
        ...base,
        stored: override({
          document: {
            merges: [
              {
                path: 'CONTRIBUTING.md',
                sections: [{ action: 'after', heading: '## Usage', content: 'Read this' }],
              },
            ],
          },
        }),
        onSave,
      });

      await fireEvent.click(screen.getByRole('radio', { name: 'Append to document' }));
      await save();

      expect(sent[0].document.merges).toEqual([
        { path: 'CONTRIBUTING.md', sections: [{ action: 'append', content: 'Read this' }] },
      ]);
    });

    /*
     * Markdown is edited by its headings, not by keys and lists, and a spec
     * carrying both is refused. A row repointed at a `.md` file would otherwise
     * save what it held as a JSON row and be refused by the planner instead.
     */
    it('leaves the keys behind when a row becomes a Markdown row', async () => {
      const { sent, onSave } = saved();
      render(RepositorySyncPane, {
        ...base,
        stored: override({
          document: {
            merges: [
              {
                path: 'renovate.json',
                overrides: { timezone: 'Europe/Warsaw' },
                arrays: [{ path: '$.extends', strategy: 'append' }],
                deduplicate: true,
              },
            ],
          },
        }),
        onSave,
      });

      await fireEvent.change(screen.getByLabelText('File'), {
        target: { value: 'CONTRIBUTING.md' },
      });
      await save();

      expect(sent[0].document.merges).toEqual([{ path: 'CONTRIBUTING.md' }]);
    });

    it('keeps a key a newer version of the service wrote on a merge', async () => {
      const { sent, onSave } = saved();
      render(RepositorySyncPane, {
        ...base,
        stored: override({
          document: { merges: [{ path: 'renovate.json', rewrites_later: ['something'] }] },
        }),
        onSave,
      });

      await fireEvent.click(screen.getByRole('button', { name: 'Add a list rule' }));
      await fireEvent.change(screen.getByLabelText('List'), { target: { value: '$.extends' } });
      await save();

      expect(sent[0].document.merges).toEqual([
        {
          path: 'renovate.json',
          rewrites_later: ['something'],
          arrays: [{ path: '$.extends', strategy: 'append' }],
        },
      ]);
    });
  });
});

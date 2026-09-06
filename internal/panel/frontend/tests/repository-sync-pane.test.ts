// @vitest-environment jsdom
import { EditorView } from '@codemirror/view';
import { EditorState } from '@codemirror/state';
import { tick } from 'svelte';
import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import RepositorySyncPane from '../src/lib/components/RepositorySyncPane.svelte';
import {
  adoptSyncOverrideSettings,
  cloneSyncOverrideEditorEnvelope,
  stageSyncOverrideControl,
  syncOverrideBatchInput,
  syncOverrideDraftEnvelope,
  type SyncOverrideControlId,
  type SyncOverrideEditorEnvelope,
} from '../src/lib/repository-sync-override-settings';
import { SettingsDraftRegistry } from '../src/lib/settings-drafts.svelte';
import type { SyncOverride } from '../src/lib/types';

function codeView(label: string, index = 0): EditorView | null {
  let matching = 0;
  for (const host of document.querySelectorAll('.code-editor')) {
    const content = host.shadowRoot?.querySelector<HTMLElement>(`[aria-label="${label}"]`);
    if (content && matching++ === index) return EditorView.findFromDOM(content);
  }
  return null;
}

async function writeCode(label: string, text: string, index = 0): Promise<void> {
  const view = codeView(label, index);
  if (!view) throw new Error(`Editor ${label} not found`);
  view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text } });
  await tick();
}

/** The controls measure themselves to place a thumb; jsdom does not. */
class TestResizeObserver {
  observe(): void {}
  disconnect(): void {}
}

/**
 * What this pane stages is the only thing that makes a repository's own
 * customization survive a sync. Drop it and the plain template is written over
 * exactly the file it described - the failure this whole port exists to stop.
 *
 * Every edit publishes a complete restart-safe envelope. Validation remains
 * local, while the shared composer decides whether Save may proceed.
 */
describe('RepositorySyncPane [Component]', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.stubGlobal('ResizeObserver', TestResizeObserver);
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

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
    const sent: SyncOverrideEditorEnvelope[] = [];
    const controls: SyncOverrideControlId[] = [];

    return {
      sent,
      controls,
      onChange: (next: SyncOverrideEditorEnvelope, control: SyncOverrideControlId) => {
        sent.splice(0, sent.length, next);
        controls.push(control);
      },
    };
  }

  /** A fixed clock, so a relative time reads the same on every run. */
  const now = Date.parse('2026-08-09T10:00:00Z');

  const base = { repositoryId: 'repo-1', readOnly: false, now, onChange: () => {} };

  /** Flushes control internals that use timers for visual state. */
  async function rest(): Promise<void> {
    await vi.advanceTimersByTimeAsync(1_000);
  }

  async function openMergeRules(): Promise<void> {
    await fireEvent.click(screen.getByRole('button', { name: 'Merge rules' }));
    await tick();
    expect(screen.getByRole('dialog', { name: 'Merge rules' })).toBeTruthy();
  }

  /** Adds one leave-alone pattern through the in-place entry editor. */
  async function addExclude(value: string): Promise<void> {
    await fireEvent.click(screen.getByRole('button', { name: 'Add' }));
    const input = screen.getByLabelText('Pattern');
    await fireEvent.input(input, { target: { value } });
    await fireEvent.keyDown(input, { key: 'Enter' });
  }

  it.each(['1e400', '-1e400', '1e-400', '-0'])(
    'stages and reopens exact file content containing %s',
    async (literal) => {
      const values = new Map<string, string>();
      const storage = {
        getItem: (key: string) => values.get(key) ?? null,
        setItem: (key: string, value: string) => {
          values.set(key, value);
        },
      };
      const stored = override({
        document: {
          merges: [
            { path: 'renovate.json', overrides: { id: JSON.rawJSON(literal), flag: false } },
          ],
        },
      });
      const first = new SettingsDraftRegistry({ storage, writerId: 'first' });
      first.hydrate('viewer');
      adoptSyncOverrideSettings(first, 'target', 'repo-1', stored);
      const component = render(RepositorySyncPane, {
        ...base,
        stored,
        onChange: (next, control) => {
          expect(stageSyncOverrideControl(first, 'target', 'repo-1', stored, next, control)).toBe(
            true,
          );
        },
      });
      await writeCode('Content adjustments', `{"id":${literal},"flag":true}`);
      expect(first.dirtyControlCount).toBe(1);
      component.unmount();
      const second = new SettingsDraftRegistry({ storage, writerId: 'second' });
      second.hydrate('viewer');
      const envelope = syncOverrideDraftEnvelope(second, 'target', 'repo-1', stored);
      render(RepositorySyncPane, { ...base, stored, envelope });
      expect(codeView('Content adjustments')?.state.doc.toString()).toBe(
        `{"id":${literal},"flag":true}`,
      );
      const batch = syncOverrideBatchInput('repo-1', stored.revision, envelope);
      expect(batch.ok).toBe(true);
      if (batch.ok) expect(JSON.stringify(batch.input.document)).toContain(`"id":${literal}`);
    },
  );

  it.each(['{"id":1,"id":2}', '{"nested":{"id":1,"id":2}}', '{"id":1,"\\u0069d":2}'])(
    'preserves duplicate-key draft text and blocks its save: %s',
    async (text) => {
      const stored = override({
        document: { merges: [{ path: 'renovate.json', overrides: { id: 0 } }] },
      });
      const values = new Map<string, string>();
      const storage = {
        getItem: (key: string) => values.get(key) ?? null,
        setItem: (key: string, value: string) => {
          values.set(key, value);
        },
      };
      const first = new SettingsDraftRegistry({ storage, writerId: 'first' });
      first.hydrate('viewer');
      adoptSyncOverrideSettings(first, 'target', 'repo-1', stored);
      const component = render(RepositorySyncPane, {
        ...base,
        stored,
        onChange: (next, control) => {
          expect(stageSyncOverrideControl(first, 'target', 'repo-1', stored, next, control)).toBe(
            true,
          );
        },
      });
      await writeCode('Content adjustments', text);
      expect(first.dirtyControlCount).toBe(1);
      component.unmount();
      const second = new SettingsDraftRegistry({ storage, writerId: 'second' });
      second.hydrate('viewer');
      const envelope = syncOverrideDraftEnvelope(second, 'target', 'repo-1', stored);
      expect(envelope.override_texts).toEqual([text]);
      expect(syncOverrideBatchInput('repo-1', stored.revision, envelope).ok).toBe(false);
      render(RepositorySyncPane, { ...base, stored, envelope });
      expect(codeView('Content adjustments')?.state.doc.toString()).toBe(text);
      await writeCode('Content adjustments', '{"id":2}');
      expect(codeView('Content adjustments')?.state.doc.toString()).toBe('{"id":2}');
    },
  );

  it('shows when a repository has no content adjustments', () => {
    render(RepositorySyncPane, { ...base, stored: override() });

    expect(screen.getByText('No content adjustments for this repository')).toBeTruthy();
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

    expect(codeView('Content adjustments')?.state.doc.toString()).toContain('Europe/Warsaw');
  });

  it('sends an adjustment somebody wrote', async () => {
    const { sent, onChange } = saved();
    render(RepositorySyncPane, { ...base, stored: override(), onChange });

    await fireEvent.click(screen.getByRole('button', { name: 'Adjust a file' }));
    await fireEvent.input(screen.getByLabelText('File'), {
      target: { value: 'renovate.json' },
    });
    await writeCode('Content adjustments', '{"timezone": "Europe/Warsaw"}');
    await rest();

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
  it('keeps malformed JSON staged while reporting it locally', async () => {
    const { sent, controls, onChange } = saved();
    render(RepositorySyncPane, {
      ...base,
      stored: override({ document: { merges: [{ path: 'renovate.json' }] } }),
      onChange,
    });

    await writeCode('Content adjustments', '{"timezone": ');
    await rest();

    expect(screen.getByRole('alert').textContent).toContain('Enter a valid JSON object');
    expect(sent[0].override_texts).toEqual(['{"timezone": ']);
    expect(controls.at(-1)).toBe('repositories.repo-1.sync.files.document');
  });

  it('sends the files this repository wants left alone', async () => {
    const { sent, onChange } = saved();
    render(RepositorySyncPane, { ...base, stored: override(), onChange });

    await addExclude('renovate.json');
    await addExclude('CONTRIBUTING.md');
    await rest();

    expect(sent.at(-1)?.document.excludes).toEqual(['renovate.json', 'CONTRIBUTING.md']);
  });

  /**
   * Three states rather than two. "Inherits, and the workspace says no" and
   * "this repository says no" are different answers that stop being the same
   * the moment the workspace changes its mind.
   */
  it('sends nothing about enablement while the repository inherits', async () => {
    const { sent, onChange } = saved();
    render(RepositorySyncPane, { ...base, stored: override(), onChange });

    await addExclude('LICENSE');
    await rest();

    expect(sent[0].enabled).toBeNull();
  });

  it('saves nothing until something changes', async () => {
    const { sent, onChange } = saved();
    render(RepositorySyncPane, { ...base, stored: override(), onChange });

    await rest();

    expect(sent).toHaveLength(0);
  });

  /**
   * Two documents that would be saved the same way have to compare the same
   * way, whatever order their keys arrived in. Comparing the raw text put Save
   * live the moment the page loaded, for a document nobody had touched.
   */
  it('saves nothing for a document whose keys arrived in another order', async () => {
    const { sent, onChange } = saved();
    render(RepositorySyncPane, {
      ...base,
      stored: override({
        document: {
          excludes: ['LICENSE'],
          merges: [{ path: 'renovate.json', overrides: { timezone: 'UTC' } }],
        },
      }),
      onChange,
    });

    await rest();

    expect(sent).toHaveLength(0);
  });

  /**
   * Anything a later version adds is stored in the same document, and a form
   * that rebuilt it from its own controls would drop every key it has no
   * control for.
   */
  it('carries through a key it has no control for', async () => {
    const { sent, onChange } = saved();
    render(RepositorySyncPane, {
      ...base,
      stored: override({ document: { something_later: { deep: true } } }),
      onChange,
    });

    await addExclude('LICENSE');
    await rest();

    expect(sent[0].document.something_later).toEqual({ deep: true });
  });

  /**
   * A document this version cannot read renders as a repository adjusting
   * nothing, which is what somebody would then save over.
   */
  it('changes nothing while the stored document cannot be read', () => {
    render(RepositorySyncPane, { ...base, stored: override({ unreadable: true }) });

    expect(screen.getByRole('alert').textContent).toContain('cannot read');
    expect(screen.getByRole('button', { name: 'Adjust a file' }).hasAttribute('disabled')).toBe(
      true,
    );
  });

  it('offers nothing to change to somebody who may only read', () => {
    render(RepositorySyncPane, {
      ...base,
      readOnly: true,
      stored: override({ document: { merges: [{ path: 'renovate.json' }] } }),
    });

    expect(screen.queryByRole('button', { name: 'Adjust a file' })).toBeNull();
    expect(codeView('Content adjustments')?.state.facet(EditorState.readOnly)).toBe(true);
  });

  it('undoes an invalid code edit without losing the saved adjustment', async () => {
    const { sent, onChange } = saved();
    render(RepositorySyncPane, {
      ...base,
      onChange,
      stored: override({
        document: { merges: [{ path: 'renovate.json', overrides: { timezone: 'UTC' } }] },
      }),
    });

    await writeCode('Content adjustments', '{"timezone":');
    expect(screen.getByRole('alert').textContent).toContain('Enter a valid JSON object');
    await fireEvent.click(screen.getByRole('button', { name: 'Undo' }));
    await tick();

    expect(screen.queryByRole('alert')).toBeNull();
    expect(sent[0].document.merges).toEqual([
      { path: 'renovate.json', overrides: { timezone: 'UTC' } },
    ]);
    expect(JSON.parse(codeView('Content adjustments')?.state.doc.toString() ?? '')).toEqual({
      timezone: 'UTC',
    });
  });

  it('keeps undo history with its file when an earlier adjustment is removed', async () => {
    const { sent, onChange } = saved();
    const rendered = render(RepositorySyncPane, {
      ...base,
      onChange,
      stored: override({
        document: {
          merges: [
            { path: 'a.json', overrides: { a: 1 } },
            { path: 'b.json', overrides: { b: 2 } },
          ],
        },
      }),
    });
    const remaining = codeView('Content adjustments', 1);
    await writeCode('Content adjustments', '{"b":3}', 1);
    await rendered.rerender({ envelope: cloneSyncOverrideEditorEnvelope(sent[0]) });
    expect(codeView('Content adjustments', 1)).toBe(remaining);
    await fireEvent.click(screen.getByRole('button', { name: 'Remove adjustment for a.json' }));
    await rendered.rerender({ envelope: cloneSyncOverrideEditorEnvelope(sent[0]) });
    expect(codeView('Content adjustments')).toBe(remaining);
    await fireEvent.click(screen.getByRole('button', { name: 'Undo' }));
    await tick();
    expect(JSON.stringify(sent[0].document.merges)).toBe('[{"path":"b.json","overrides":{"b":2}}]');

    // Cached navigation can reuse this pane for a different repository with
    // exactly the same document. Its history still belongs to that repository.
    await rendered.rerender({
      repositoryId: 'repo-2',
      envelope: cloneSyncOverrideEditorEnvelope(sent[0]),
    });
    expect(codeView('Content adjustments')).not.toBe(remaining);
    expect(screen.queryByRole('button', { name: 'Undo' })).toBeNull();

    // Discard is a new document, not another local edit in the same history.
    await rendered.rerender({ envelope: undefined });
    expect(codeView('Content adjustments', 1)).not.toBe(remaining);
    expect(screen.queryByRole('button', { name: 'Undo' })).toBeNull();
  });

  it.each(['renovate.jsonc', 'config.toml'])(
    'edits %s content adjustments as JSON',
    async (path) => {
      const { sent, onChange } = saved();
      render(RepositorySyncPane, {
        ...base,
        onChange,
        stored: override({ document: { merges: [{ path, overrides: { enabled: true } }] } }),
      });

      await writeCode('Content adjustments', '{"enabled":false}');
      expect(screen.queryByRole('alert')).toBeNull();
      expect(sent[0].document.merges).toEqual([{ path, overrides: { enabled: false } }]);
      expect(sent[0].override_texts).toEqual(['{"enabled":false}']);
    },
  );

  it('keeps invalid rules and editor history when the inspector closes', async () => {
    const { sent, onChange } = saved();
    const rendered = render(RepositorySyncPane, {
      ...base,
      onChange,
      stored: override({
        document: { merges: [{ path: 'renovate.json', overrides: { entries: [1] } }] },
      }),
    });
    await writeCode('Content adjustments', '{"entries":[2]}');
    const editor = codeView('Content adjustments');
    await openMergeRules();
    await fireEvent.click(screen.getByRole('button', { name: 'Add a list rule' }));
    expect(screen.getByRole('alert').textContent).toContain('names no list');
    const draft = cloneSyncOverrideEditorEnvelope(sent[0]);

    await fireEvent.click(screen.getByRole('button', { name: 'Done' }));
    await tick();
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(codeView('Content adjustments')).toBe(editor);
    expect(screen.getByRole('button', { name: 'Undo' })).toBeTruthy();
    expect(sent[0]).toEqual(draft);
    expect(screen.getByRole('alert').textContent).toContain('names no list');

    await openMergeRules();
    expect((screen.getByLabelText('List') as HTMLInputElement).value).toBe('');
    expect(screen.getByRole('alert').textContent).toContain('names no list');

    // An external discard changes draft identity and closes the old inspector.
    await rendered.rerender({
      envelope: {
        enabled: null,
        document: { merges: [{ path: 'other.json', overrides: { kept: true } }] },
        override_texts: ['{"kept":true}'],
      },
    });
    await tick();
    expect(screen.queryByRole('dialog')).toBeNull();
    expect((screen.getByLabelText('File') as HTMLInputElement).value).toBe('other.json');
  });

  it('keeps undo history with its Markdown section after removing an earlier section', async () => {
    const { sent, onChange } = saved();
    render(RepositorySyncPane, {
      ...base,
      onChange,
      stored: override({
        document: {
          merges: [
            {
              path: 'README.md',
              sections: [
                { action: 'append', content: 'First section' },
                { action: 'append', content: 'Second section' },
              ],
            },
          ],
        },
      }),
    });
    const remaining = codeView('What this repository writes', 1);
    await writeCode('What this repository writes', 'Changed second section', 1);
    await fireEvent.click(screen.getAllByRole('button', { name: 'Remove' })[0]);
    expect(codeView('What this repository writes')).toBe(remaining);
    await fireEvent.click(screen.getByRole('button', { name: 'Undo' }));
    await tick();
    expect(sent[0].document.merges).toEqual([
      {
        path: 'README.md',
        sections: [{ action: 'append', content: 'Second section' }],
      },
    ]);
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

    /* Scoped by class: the saved receipt is a quiet status of its own now. */
    const notice = document.querySelector('.sync-pane-standdown');
    expect(notice?.textContent).toContain('are not being synced here');
    expect(notice?.textContent).toContain('docs is not a directory in this repository');

    // And when it was found, so a fix saved a minute ago can be told from one
    // this notice already knows about.
    expect(notice?.textContent).toContain('3 minutes ago');
  });

  it('says nothing where the planner found nothing wrong', () => {
    render(RepositorySyncPane, { ...base, stored: override() });

    expect(document.querySelector('.sync-pane-standdown')).toBeNull();
  });

  /**
   * The three things the merge engine has always implemented and this pane
   * could not say. Nine of the organization's thirteen repositories adjust a
   * template, and six of them do it with a list rule or a heading - so without
   * these the cutover writes the plain template over exactly those six.
   */
  describe('what a repository does beyond setting keys', () => {
    it('appends to a list rather than replacing it', async () => {
      const { sent, onChange } = saved();
      render(RepositorySyncPane, {
        ...base,
        stored: override({
          document: {
            merges: [{ path: 'renovate.json', overrides: { packageRules: [{ id: 'go' }] } }],
          },
        }),
        onChange,
      });

      await openMergeRules();
      await fireEvent.click(screen.getByRole('button', { name: 'Add a list rule' }));
      await fireEvent.input(screen.getByLabelText('List'), {
        target: { value: '$.packageRules' },
      });
      await rest();

      expect(sent[0].document.merges).toEqual([
        {
          path: 'renovate.json',
          overrides: { packageRules: [{ id: 'go' }] },
          arrays: [{ path: '$.packageRules', strategy: 'append' }],
        },
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

      await openMergeRules();
      expect(screen.queryByText('Drop repeated entries')).toBeNull();

      await fireEvent.click(screen.getByRole('button', { name: 'Add a list rule' }));

      expect(screen.getByText('Drop repeated entries')).toBeTruthy();
    });

    it('writes deduplication beside the rule it belongs to', async () => {
      const { sent, onChange } = saved();
      render(RepositorySyncPane, {
        ...base,
        stored: override({
          document: {
            merges: [
              {
                path: 'renovate.json',
                overrides: { extends: ['config:base'] },
                arrays: [{ path: '$.extends', strategy: 'append' }],
              },
            ],
          },
        }),
        onChange,
      });

      await openMergeRules();
      await fireEvent.click(
        screen.getByRole('checkbox', { name: 'Drop repeated entries from renovate.json' }),
      );
      await rest();

      expect(sent[0].document.merges).toEqual([
        {
          path: 'renovate.json',
          overrides: { extends: ['config:base'] },
          arrays: [{ path: '$.extends', strategy: 'append' }],
          deduplicate: true,
        },
      ]);
    });

    it('never writes deduplication without the rule it belongs to', async () => {
      const { sent, onChange } = saved();
      render(RepositorySyncPane, {
        ...base,
        stored: override({
          document: {
            merges: [
              {
                path: 'renovate.json',
                overrides: { extends: ['config:base'] },
                arrays: [{ path: '$.extends', strategy: 'append' }],
                deduplicate: true,
              },
            ],
          },
        }),
        onChange,
      });

      await openMergeRules();
      await fireEvent.click(screen.getByRole('button', { name: 'Remove list rule $.extends' }));
      await rest();

      expect(sent[0].document.merges).toEqual([
        { path: 'renovate.json', overrides: { extends: ['config:base'] } },
      ]);
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

      expect(codeView('Content adjustments')).toBeNull();
      expect(screen.getByRole('button', { name: 'Edit a section' })).toBeTruthy();
    });

    it('writes a heading with the marks the document writes it with', async () => {
      const { sent, onChange } = saved();
      render(RepositorySyncPane, {
        ...base,
        stored: override({ document: { merges: [{ path: 'CONTRIBUTING.md' }] } }),
        onChange,
      });

      await fireEvent.click(screen.getByRole('button', { name: 'Edit a section' }));
      await fireEvent.input(screen.getByLabelText('Heading'), {
        target: { value: '### Prerequisites' },
      });
      await writeCode('What this repository writes', '### Project setup');
      await rest();

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
      const { sent, onChange } = saved();
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
        onChange,
      });

      await fireEvent.click(screen.getByRole('radio', { name: 'Append to document' }));
      await rest();

      expect(sent[0].document.merges).toEqual([
        { path: 'CONTRIBUTING.md', sections: [{ action: 'append', content: 'Read this' }] },
      ]);
    });

    /*
     * Markdown is edited by its headings, not by keys and lists, and a spec
     * carrying both is refused. A row repointed at a `.md` file would otherwise
     * save what it held as a JSON row and be refused by the planner instead.
     */
    it('holds a row retyped to Markdown until a section says how', async () => {
      const { sent, onChange } = saved();
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
        onChange,
      });

      await fireEvent.input(screen.getByLabelText('File'), {
        target: { value: 'CONTRIBUTING.md' },
      });
      await rest();

      /* A bare Markdown row says nothing yet, so the rest saves nothing and
         the refusal is spoken where the row is. The keys being left behind is
         proven by the sibling below, which completes the section and saves. */
      expect(sent).toHaveLength(1);
      expect(screen.getByRole('alert').textContent).toContain('no section says how');
    });

    /*
     * The strategy control cannot offer a pair the engine refuses, but retyping
     * the path arrives at it from the other side: `validateStrategy` rejects a
     * Markdown strategy on a `.json` path and a deep merge on a `.md` one.
     */
    it('drops a Markdown strategy when the path stops being Markdown', async () => {
      render(RepositorySyncPane, {
        ...base,
        stored: override({
          document: {
            merges: [
              {
                path: 'CONTRIBUTING.md',
                strategy: 'markdown',
                sections: [{ action: 'after', heading: '## Usage', content: 'Read this' }],
              },
            ],
          },
        }),
      });

      await fireEvent.input(screen.getByLabelText('File'), {
        target: { value: 'renovate.json' },
      });

      // The row is composed by keys again, which it can only be once the
      // Markdown strategy it was carrying is gone.
      expect(codeView('Content adjustments')).toBeTruthy();
      expect(screen.queryByRole('button', { name: 'Edit a section' })).toBeNull();
    });

    it('drops a keys-and-lists strategy when the path becomes Markdown', async () => {
      render(RepositorySyncPane, {
        ...base,
        stored: override({
          document: {
            merges: [
              {
                path: 'renovate.json',
                strategy: 'deep-merge',
                overrides: { timezone: 'Europe/Warsaw' },
              },
            ],
          },
        }),
      });

      await fireEvent.input(screen.getByLabelText('File'), {
        target: { value: 'CONTRIBUTING.md' },
      });

      expect(screen.getByRole('button', { name: 'Edit a section' })).toBeTruthy();
      expect(codeView('Content adjustments')).toBeNull();
    });

    /* A strategy the new path still allows is the row's answer, not noise. */
    it('keeps a strategy the new path still allows', async () => {
      const { sent, onChange } = saved();
      render(RepositorySyncPane, {
        ...base,
        stored: override({
          document: {
            merges: [
              {
                path: 'renovate.json',
                strategy: 'shallow-merge',
                overrides: { timezone: 'Europe/Warsaw' },
              },
            ],
          },
        }),
        onChange,
      });

      await fireEvent.input(screen.getByLabelText('File'), {
        target: { value: '.github/settings.yml' },
      });
      await rest();

      expect(sent[0].document.merges).toEqual([
        {
          path: '.github/settings.yml',
          strategy: 'shallow-merge',
          overrides: { timezone: 'Europe/Warsaw' },
        },
      ]);
    });

    /*
     * Markdown is edited by its headings, not by keys and lists, and a spec
     * carrying both is refused. Saved once the row says what it does, so the
     * document proves the keys were left behind rather than carried.
     */
    it('leaves the keys behind when a row becomes a Markdown row', async () => {
      const { sent, onChange } = saved();
      render(RepositorySyncPane, {
        ...base,
        stored: override({
          document: {
            merges: [
              {
                path: 'renovate.json',
                overrides: { extends: ['config:base'] },
                arrays: [{ path: '$.extends', strategy: 'append' }],
                deduplicate: true,
              },
            ],
          },
        }),
        onChange,
      });

      await fireEvent.input(screen.getByLabelText('File'), {
        target: { value: 'CONTRIBUTING.md' },
      });
      await fireEvent.click(screen.getByRole('button', { name: 'Edit a section' }));
      await fireEvent.input(screen.getByLabelText('Heading'), {
        target: { value: '### Prerequisites' },
      });
      await writeCode('What this repository writes', 'Run `mise install`');
      await rest();

      expect(sent[0].document.merges).toEqual([
        {
          path: 'CONTRIBUTING.md',
          sections: [
            { action: 'after', heading: '### Prerequisites', content: 'Run `mise install`' },
          ],
        },
      ]);
    });

    /**
     * Every merge is validated on save, and `Spec.Empty()` does not rescue a
     * half-filled row: that short circuit lives in `Apply`, not on the save
     * path. Without these, each new Add button makes a row the server refuses
     * the moment it exists, answered by one flat sentence at the top of a pane
     * that can hold several files.
     */
    describe('refusing what the engine would refuse', () => {
      function refusal(): string {
        return screen.getByRole('alert').textContent ?? '';
      }

      it('will not save a list rule with no list named', async () => {
        const { sent, onChange } = saved();
        render(RepositorySyncPane, {
          ...base,
          onChange,
          stored: override({
            document: { merges: [{ path: 'renovate.json', overrides: { a: [1] } }] },
          }),
        });

        await openMergeRules();
        await fireEvent.click(screen.getByRole('button', { name: 'Add a list rule' }));

        expect(refusal()).toContain('names no list');
        await rest();
        expect(sent).toHaveLength(1);
      });

      /*
       * A rule says what to do with the repository's list where the template
       * has one, so a rule whose path no override sets has no list to work
       * with - for every template, always.
       */
      it('will not save a list rule the overrides do not set', async () => {
        const { sent, onChange } = saved();
        render(RepositorySyncPane, {
          ...base,
          onChange,
          stored: override({
            document: {
              merges: [
                {
                  path: 'renovate.json',
                  overrides: { extends: ['config:base'] },
                  arrays: [{ path: '$.packageRules', strategy: 'append' }],
                },
              ],
            },
          }),
        });

        expect(refusal()).toContain('No override sets $.packageRules');
        await rest();
        expect(sent).toHaveLength(0);
      });

      it('will not save a list rule pointing at something that is not a list', async () => {
        const { sent, onChange } = saved();
        render(RepositorySyncPane, {
          ...base,
          onChange,
          stored: override({
            document: {
              merges: [
                {
                  path: 'renovate.json',
                  overrides: { timezone: 'Europe/Warsaw' },
                  arrays: [{ path: '$.timezone', strategy: 'append' }],
                },
              ],
            },
          }),
        });

        expect(refusal()).toContain('is not a list');
        await rest();
        expect(sent).toHaveLength(0);
      });

      it('will not save two rules for one list', async () => {
        const { sent, onChange } = saved();
        render(RepositorySyncPane, {
          ...base,
          onChange,
          stored: override({
            document: {
              merges: [
                {
                  path: 'renovate.json',
                  overrides: { extends: ['config:base'] },
                  arrays: [
                    { path: '$.extends', strategy: 'append' },
                    { path: '$.extends', strategy: 'prepend' },
                  ],
                },
              ],
            },
          }),
        });

        expect(refusal()).toContain('two rules for $.extends');
        await rest();
        expect(sent).toHaveLength(0);
      });

      /* A shallow merge replaces a top-level key whole, so nothing below one
         is ever merged and a rule pointing there describes work that cannot
         happen. */
      it('will not save a nested list rule under a shallow merge', async () => {
        const { sent, onChange } = saved();
        render(RepositorySyncPane, {
          ...base,
          onChange,
          stored: override({
            document: {
              merges: [
                {
                  path: 'renovate.json',
                  strategy: 'shallow-merge',
                  overrides: { host: { rules: [1] } },
                  arrays: [{ path: '$.host.rules', strategy: 'append' }],
                },
              ],
            },
          }),
        });

        expect(refusal()).toContain('below the top level');
        await rest();
        expect(sent).toHaveLength(0);
      });

      it('will not save a section with no heading', async () => {
        const { sent, onChange } = saved();
        render(RepositorySyncPane, {
          ...base,
          onChange,
          stored: override({ document: { merges: [{ path: 'CONTRIBUTING.md' }] } }),
        });

        await fireEvent.click(screen.getByRole('button', { name: 'Edit a section' }));

        expect(refusal()).toContain('needs the heading it addresses');
        await rest();
        expect(sent).toHaveLength(1);
      });

      it('will not save a patch that finds nothing', async () => {
        const { sent, onChange } = saved();
        render(RepositorySyncPane, {
          ...base,
          onChange,
          stored: override({
            document: {
              merges: [
                {
                  path: 'CONTRIBUTING.md',
                  sections: [{ action: 'patch', heading: '### Making Changes' }],
                },
              ],
            },
          }),
        });

        expect(refusal()).toContain('substitutes nothing');
        await rest();
        expect(sent).toHaveLength(0);
      });

      it('will not save a Markdown row that says nothing', async () => {
        const { sent, onChange } = saved();
        render(RepositorySyncPane, {
          ...base,
          onChange,
          stored: override({ document: { merges: [{ path: 'CONTRIBUTING.md' }] } }),
        });

        expect(refusal()).toContain('no section says how');
        await rest();
        expect(sent).toHaveLength(0);
      });

      it('will not save a row that merges nothing', async () => {
        const { sent, onChange } = saved();
        render(RepositorySyncPane, {
          ...base,
          onChange,
          stored: override({ document: { merges: [{ path: 'renovate.json' }] } }),
        });

        expect(refusal()).toContain('sets nothing and has no list rule');
        await rest();
        expect(sent).toHaveLength(0);
      });

      it('will not save a file with no extension the engine can merge', async () => {
        const { sent, onChange } = saved();
        render(RepositorySyncPane, {
          ...base,
          onChange,
          stored: override({
            document: { merges: [{ path: 'LICENSE', overrides: { a: 1 } }] },
          }),
        });

        expect(refusal()).toContain('no extension this can merge');
        await rest();
        expect(sent).toHaveLength(0);
      });

      it('will not save one file adjusted twice', async () => {
        const { sent, onChange } = saved();
        render(RepositorySyncPane, {
          ...base,
          onChange,
          stored: override({
            document: {
              merges: [
                { path: 'renovate.json', overrides: { a: 1 } },
                { path: 'renovate.json', overrides: { b: 2 } },
              ],
            },
          }),
        });

        expect(refusal()).toContain('adjusted twice');
        await rest();
        expect(sent).toHaveLength(0);
      });
    });

    it('keeps a key a newer version of the service wrote on a merge', async () => {
      const { sent, onChange } = saved();
      render(RepositorySyncPane, {
        ...base,
        stored: override({
          document: {
            merges: [
              {
                path: 'renovate.json',
                overrides: { extends: ['config:base'] },
                rewrites_later: ['something'],
              },
            ],
          },
        }),
        onChange,
      });

      await openMergeRules();
      await fireEvent.click(screen.getByRole('button', { name: 'Add a list rule' }));
      await fireEvent.input(screen.getByLabelText('List'), { target: { value: '$.extends' } });
      await rest();

      expect(sent[0].document.merges).toEqual([
        {
          path: 'renovate.json',
          overrides: { extends: ['config:base'] },
          rewrites_later: ['something'],
          arrays: [{ path: '$.extends', strategy: 'append' }],
        },
      ]);
    });
  });
});

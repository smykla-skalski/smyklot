// @vitest-environment jsdom
import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import SyncRulesetPage from '../src/lib/components/SyncRulesetPage.svelte';
import SyncRulesetsPage from '../src/lib/components/SyncRulesetsPage.svelte';
import { formatJson, parseJson, type JsonValue } from '../src/lib/merge';
import type { SyncConfig, SyncRuleset } from '../src/lib/types';
import { chooseOption } from './support/select';
import { tick } from 'svelte';
import RulesetRuleEditor from '../src/lib/components/RulesetRuleEditor.svelte';
import type { BypassActorDirectory } from '../src/lib/types';

class TestResizeObserver {
  observe(): void {}
  disconnect(): void {}
}

/**
 * The two ruleset pages: the list of named objects and one object's editor.
 * The promises worth holding: a ruleset is written by replacement so what a
 * page does not know must SURVIVE a save untouched, a rule turned on
 * arrives in the smallest shape GitHub accepts, and a new ruleset is born
 * disabled on the default branch rather than active with no rules.
 */
describe('the ruleset pages [Component]', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', TestResizeObserver);
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  afterEach(() => vi.unstubAllGlobals());

  function config(document: Record<string, unknown>, over: Partial<SyncConfig> = {}): SyncConfig {
    return {
      kind: 'rulesets',
      enabled: true,
      labels: [],
      allow_removal: false,
      excludes: [],
      revision: 1,
      updated_by: 'bart',
      updated_at: new Date(0).toISOString(),
      digest: '',
      document,
      unreadable: false,
      unavailable: '',
      ...over,
    };
  }

  /* The detail page still sits under Rulesets and says so with a crumb; the
     list page IS the tree row, so its way back is the tree. */
  const shared = {
    readOnly: false,
    problem: null,
    sectionHref: () => '#',
    onOpenSection: () => {},
    onChangeDocument: () => {},
  };

  const listShared = {
    readOnly: false,
    problem: null,
    nowMs: Date.UTC(2026, 7, 18, 12, 0, 0),
    onChangeDocument: () => {},
    plan: null,
    rulesetHref: (name: string) => `#/${name}`,
    onOpenRuleset: () => {},
    onToggleEnabled: () => {},
  };

  function savedProtection(): SyncRuleset {
    return {
      name: 'main-protection',
      target: 'branch',
      enforcement: 'active',
      conditions: { include: ['~DEFAULT_BRANCH'], exclude: [] },
      bypass_actors: [
        { actor_id: 0, actor_type: 'OrganizationAdmin', bypass_mode: 'always' },
        { actor_id: 9, actor_type: 'Integration', bypass_mode: 'pull_request' },
      ],
      rules: {
        pull_request: {
          required_approving_review_count: 1,
          allowed_merge_methods: ['squash', 'merge'],
        },
        deletion: true,
      },
    };
  }

  it('marks only the changed actor after the draft normalizes object key order', async () => {
    const saved = savedProtection();
    const edited = structuredClone(saved);
    edited.bypass_actors![1]!.bypass_mode = 'always';
    const normalized = parseJson(
      formatJson({ rulesets: [edited] } as unknown as JsonValue),
    ) as Record<string, unknown>;
    const { rerender } = render(SyncRulesetPage, {
      ...shared,
      name: saved.name,
      config: config(normalized),
      savedDocument: parseJson(formatJson({ rulesets: [saved] } as unknown as JsonValue)) as Record<
        string,
        unknown
      >,
      dirtyDocument: true,
    });

    expect(document.querySelectorAll('.card.is-unsaved')).toHaveLength(1);
    expect(document.querySelector('.card.is-unsaved .card-title')?.textContent).toBe('Bypass list');
    expect(
      document.querySelectorAll('.policy-row.is-unsaved, .setting-row.is-unsaved'),
    ).toHaveLength(0);
    expect(document.querySelectorAll('.actor-row.is-unsaved')).toHaveLength(1);
    expect(document.querySelector('.actor-row.is-unsaved')?.textContent).toContain(
      'Unavailable app',
    );

    await rerender({ config: config({ rulesets: [saved] }) });
    expect(document.querySelectorAll('.is-unsaved')).toHaveLength(0);
  });

  it('tracks included and excluded branches separately and clears restored sets', async () => {
    const saved = savedProtection();
    const edited = structuredClone(saved);
    edited.conditions.include = ['release/*', '~DEFAULT_BRANCH'];
    const { rerender } = render(SyncRulesetPage, {
      ...shared,
      name: saved.name,
      config: config({ rulesets: [edited] }),
      savedDocument: { rulesets: [saved] },
      dirtyDocument: true,
    });
    expect(
      screen.getByText('Included branches').closest('.policy-row')?.getAttribute('data-unsaved'),
    ).toBe('true');
    expect(
      screen.getByText('Excluded branches').closest('.policy-row')?.getAttribute('data-unsaved'),
    ).toBeNull();

    edited.conditions.exclude = ['archive/*'];
    await rerender({ config: config({ rulesets: [structuredClone(edited)] }) });
    expect(
      screen.getByText('Excluded branches').closest('.policy-row')?.getAttribute('data-unsaved'),
    ).toBe('true');

    edited.conditions.include = saved.conditions.include;
    edited.rules.pull_request!.allowed_merge_methods.reverse();
    await rerender({ config: config({ rulesets: [structuredClone(edited)] }) });
    expect(
      screen.getByText('Included branches').closest('.policy-row')?.getAttribute('data-unsaved'),
    ).toBeNull();
    expect(
      screen
        .getByText('Require a pull request')
        .closest('.policy-row')
        ?.getAttribute('data-unsaved'),
    ).toBeNull();
    expect(document.querySelectorAll('.card.is-unsaved')).toHaveLength(1);
  });

  it('keeps unchanged rulesets neutral in the list when another ruleset changes', () => {
    const saved = savedProtection();
    const edited = { ...savedProtection(), name: 'other', enforcement: 'disabled' };
    render(SyncRulesetsPage, {
      ...listShared,
      config: config(
        parseJson(formatJson({ rulesets: [saved, edited] } as unknown as JsonValue)) as Record<
          string,
          unknown
        >,
      ),
      savedDocument: { rulesets: [saved, { ...savedProtection(), name: 'other' }] },
      dirtyDocument: true,
    });
    expect(document.querySelectorAll('.object-row.is-unsaved')).toHaveLength(1);
    expect(document.querySelector('.object-row.is-unsaved')?.textContent).toContain('other');
  });

  it('opens the actor editor from the shared Add action in the card header', async () => {
    render(SyncRulesetPage, {
      ...shared,
      name: 'main-protection',
      config: config({ rulesets: [savedProtection()] }),
    });
    const trigger = screen.getByRole('button', { name: 'Add an actor' });
    expect(trigger.closest('.card-head')).not.toBeNull();
    expect(screen.getAllByRole('button', { name: 'Add an actor' })).toHaveLength(1);
    await fireEvent.click(trigger);
    expect(await screen.findByRole('combobox', { name: 'Who' })).toBeTruthy();
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    await fireEvent.click(trigger);
    expect(screen.queryByRole('combobox', { name: 'Who' })).toBeNull();
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
  });

  it('reads the list into rows: coverage, rule count, bypass, enforcement', () => {
    render(SyncRulesetsPage, {
      ...listShared,
      config: config({
        rulesets: [
          {
            name: 'main-protection',
            target: 'branch',
            enforcement: 'active',
            conditions: { include: ['~DEFAULT_BRANCH'], exclude: [] },
            bypass_actors: [
              { actor_id: 5, actor_type: 'RepositoryRole', bypass_mode: 'always' },
              { actor_id: 9, actor_type: 'Integration', bypass_mode: 'pull_request' },
            ],
            rules: { deletion: true, non_fast_forward: true },
          },
        ],
      }),
    });

    const row = document.querySelector('.object-row') as HTMLElement;
    const text = (row.textContent ?? '').replace(/\s+/g, ' ');
    expect(text).toContain('main-protection');
    expect(text).toContain('Active');
    expect(text).toContain('default branch · 2 rules · 2 bypass actors');
  });

  it('gives a new ruleset the disabled default-branch shape and opens it', async () => {
    const sent: Array<Record<string, unknown>> = [];
    const opened: string[] = [];
    render(SyncRulesetsPage, {
      ...listShared,
      config: config({ rulesets: [] }),
      onOpenRuleset: (name: string) => {
        opened.push(name);
      },
      onChangeDocument: (document: Record<string, unknown>) => {
        sent.push(document);
      },
    });

    await fireEvent.click(screen.getByRole('button', { name: /Add a ruleset/ }));
    const input = await screen.findByLabelText('Name for the new ruleset');
    await fireEvent.input(input, { target: { value: 'release-tags' } });
    await fireEvent.keyDown(input, { key: 'Enter' });

    expect(opened).toEqual(['release-tags']);
    expect(sent[0]?.rulesets).toEqual([
      {
        name: 'release-tags',
        target: 'branch',
        enforcement: 'disabled',
        conditions: { include: ['~DEFAULT_BRANCH'], exclude: [] },
        rules: {},
      },
    ]);
  });

  it('keeps what it has no control for through a save', async () => {
    const sent: Array<Record<string, unknown>> = [];
    render(SyncRulesetPage, {
      ...shared,
      name: 'guard',
      config: config({
        some_future_key: 'kept',
        rulesets: [
          {
            name: 'guard',
            target: 'branch',
            enforcement: 'disabled',
            future_field: 'kept too',
            conditions: { include: [], exclude: [] },
            rules: {},
          },
        ],
      }),
      onChangeDocument: (document: Record<string, unknown>) => {
        sent.push(document);
      },
    });

    const active = [...document.querySelectorAll<HTMLInputElement>('input[type="radio"]')].find(
      (held) => held.value === 'active',
    );
    await fireEvent.click(active as HTMLInputElement);

    expect(sent[0]?.some_future_key).toBe('kept');
    const saved = (sent[0]?.rulesets as Array<Record<string, unknown>>)[0];
    expect(saved?.future_field).toBe('kept too');
    expect(saved?.enforcement).toBe('active');
  });

  it('gives a rule turned on the smallest shape GitHub accepts', async () => {
    const sent: Array<Record<string, unknown>> = [];
    render(SyncRulesetPage, {
      ...shared,
      name: 'guard',
      config: config({
        rulesets: [
          {
            name: 'guard',
            target: 'branch',
            enforcement: 'active',
            conditions: { include: ['~DEFAULT_BRANCH'], exclude: [] },
            rules: {},
          },
        ],
      }),
      onChangeDocument: (document: Record<string, unknown>) => {
        sent.push(document);
      },
    });

    await fireEvent.click(screen.getByRole('button', { name: /Add a rule/ }));
    const choice = screen.getByRole('button', { name: 'Require a pull request' });
    expect(choice.classList.contains('btn-add')).toBe(true);
    await fireEvent.click(choice);
    expect(sent).toHaveLength(0);
    await fireEvent.click(screen.getByRole('button', { name: 'Done' }));

    const saved = (sent[0]?.rulesets as Array<Record<string, unknown>>)[0];
    expect(saved?.rules).toEqual({
      pull_request: {
        required_approving_review_count: 1,
        allowed_merge_methods: ['merge', 'squash', 'rebase'],
      },
    });
  });

  it('switching a rule off removes it rather than writing false', async () => {
    const sent: Array<Record<string, unknown>> = [];
    render(SyncRulesetPage, {
      ...shared,
      name: 'guard',
      config: config({
        rulesets: [
          {
            name: 'guard',
            target: 'branch',
            enforcement: 'active',
            conditions: { include: ['~DEFAULT_BRANCH'], exclude: [] },
            rules: { deletion: true, non_fast_forward: true },
          },
        ],
      }),
      onChangeDocument: (document: Record<string, unknown>) => {
        sent.push(document);
      },
    });

    const row = [...document.querySelectorAll<HTMLElement>('.policy-row')].find((held) =>
      (held.textContent ?? '').includes('Restrict deletions'),
    );
    expect(row).toBeDefined();
    await fireEvent.click(within(row!).getByRole('button', { name: 'Switch the rule off' }));

    const saved = (sent[0]?.rulesets as Array<Record<string, unknown>>)[0];
    expect(saved?.rules).toEqual({ non_fast_forward: true });
  });

  it('keeps zero approvals when reopening a rule with no approval count', async () => {
    const sent = vi.fn();
    render(SyncRulesetPage, {
      ...shared,
      name: 'guard',
      config: config({
        rulesets: [
          {
            name: 'guard',
            target: 'branch',
            enforcement: 'active',
            conditions: { include: ['~DEFAULT_BRANCH'], exclude: [] },
            rules: { pull_request: { allowed_merge_methods: ['squash'] } },
          },
        ],
      }),
      onChangeDocument: sent,
    });

    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    expect(
      (screen.getByRole('spinbutton', { name: 'Approvals required' }) as HTMLInputElement).value,
    ).toBe('0');
    await fireEvent.click(screen.getByRole('button', { name: 'Done' }));
    expect(sent.mock.calls[0]![0]).toEqual(
      expect.objectContaining({
        rulesets: [
          expect.objectContaining({
            rules: { pull_request: { allowed_merge_methods: ['squash'] } },
          }),
        ],
      }),
    );
  });

  it('reads a document whose numbers are raw-JSON boxes', () => {
    // The wire read grafts a digit-preserving parse over `document`, so every
    // number in a REAL config is a null-prototype box that String() and
    // template literals throw on. The jsdom fixtures above hand plain numbers,
    // which is exactly how the page crashed in the browser while every spec
    // here stayed green - this one reads the shape the API actually delivers.
    const boxed = parseJson(
      JSON.stringify({
        rulesets: [
          {
            name: 'main-protection',
            target: 'branch',
            enforcement: 'active',
            conditions: { include: ['~DEFAULT_BRANCH'], exclude: [] },
            bypass_actors: [{ actor_id: 5, actor_type: 'RepositoryRole', bypass_mode: 'always' }],
            rules: {
              pull_request: { required_approving_review_count: 1 },
            },
          },
        ],
      }),
    ) as Record<string, unknown>;
    render(SyncRulesetPage, {
      ...shared,
      name: 'main-protection',
      config: config(boxed),
    });

    const text = (document.body.textContent ?? '').replace(/\s+/g, ' ');
    expect(text).toContain('1approval');
    expect(text).toContain('Repository admin');
  });

  it('says so on an address naming a ruleset that is gone', () => {
    render(SyncRulesetPage, {
      ...shared,
      name: 'renamed-away',
      config: config({ rulesets: [] }),
    });

    expect(document.body.textContent).toContain('No ruleset by this name');
    expect(document.querySelector('.policy-row')).toBeNull();
  });
  it.each([
    ['pull_request', { required_approving_review_count: 1, allowed_merge_methods: ['squash'] }],
    [
      'required_status_checks',
      {
        required_status_checks: [{ context: 'test', integration_id: 42, future_child: 'kept' }],
        strict_required_status_checks_policy: false,
      },
    ],
    ['update', { update_allows_fetch_and_merge: false }],
    [
      'code_scanning',
      {
        code_scanning_tools: [
          {
            tool: 'CodeQL',
            alerts_threshold: 'all',
            security_alerts_threshold: 'critical',
            future_child: 'kept',
          },
        ],
      },
    ],
  ] as const)(
    'keeps complete %s parameters when Done stages an unchanged editor',
    async (key, parameters) => {
      const sent = vi.fn();
      const rule = {
        ...parameters,
        future_parameter: parseJson('{"exact":9007199254740993,"huge":1e400}'),
      };
      render(SyncRulesetPage, {
        ...shared,
        name: 'guard',
        config: config({
          rulesets: [
            {
              name: 'guard',
              target: 'branch',
              enforcement: 'active',
              conditions: { include: [], exclude: [] },
              rules: { [key]: rule },
            },
          ],
        }),
        onChangeDocument: sent,
      });
      await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
      await fireEvent.click(screen.getByRole('button', { name: 'Done' }));
      expect(sent).toHaveBeenCalledTimes(1);
      const saved = sent.mock.calls[0]![0].rulesets[0].rules[key];
      expect(formatJson(saved)).toBe(formatJson(rule as unknown as JsonValue));
    },
  );

  it('does not create a parameterized rule until Done and Cancel leaves it absent', async () => {
    const sent = vi.fn();
    render(SyncRulesetPage, {
      ...shared,
      name: 'guard',
      config: config({
        rulesets: [
          {
            name: 'guard',
            target: 'branch',
            enforcement: 'active',
            conditions: { include: [], exclude: [] },
            rules: {},
          },
        ],
      }),
      onChangeDocument: sent,
    });
    await fireEvent.click(screen.getByRole('button', { name: /Add a rule/ }));
    await fireEvent.click(screen.getByRole('button', { name: 'Require a pull request' }));
    expect(sent).not.toHaveBeenCalled();
    await fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(sent).not.toHaveBeenCalled();
    expect(screen.queryByText('Approvals required', { exact: true })).toBeNull();
  });

  it('makes an open editor read only when write permission is removed', async () => {
    const sent = vi.fn();
    const rendered = render(SyncRulesetPage, {
      ...shared,
      name: 'guard',
      config: config({
        rulesets: [
          {
            name: 'guard',
            target: 'branch',
            enforcement: 'active',
            conditions: { include: [], exclude: [] },
            rules: { pull_request: { allowed_merge_methods: ['squash'] } },
          },
        ],
      }),
      onChangeDocument: sent,
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    await rendered.rerender({ readOnly: true });
    expect(
      (
        screen.getByRole('checkbox', {
          name: "Require a code owner's review",
        }) as HTMLInputElement
      ).disabled,
    ).toBe(true);
    expect((screen.getByRole('button', { name: 'Done' }) as HTMLButtonElement).disabled).toBe(true);
    await fireEvent.click(screen.getByRole('button', { name: 'Done' }));
    expect(sent).not.toHaveBeenCalled();
  });

  it('closes a rule editor when the selected ruleset changes', async () => {
    const make = (name: string) => ({
      name,
      target: 'branch',
      enforcement: 'active',
      conditions: { include: [], exclude: [] },
      rules: { pull_request: { allowed_merge_methods: ['squash'] } },
    });
    const sent = vi.fn();
    const rendered = render(SyncRulesetPage, {
      ...shared,
      name: 'first',
      config: config({ rulesets: [make('first'), make('second')] }),
      onChangeDocument: sent,
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    await rendered.rerender({ name: 'second' });
    expect(screen.queryByText('Approvals required', { exact: true })).toBeNull();
    expect(sent).not.toHaveBeenCalled();
  });

  it('keeps authored pinned checks through removal, re-addition and a policy edit', async () => {
    const rule = parseJson(
      '{"required_status_checks":[{"context":"test","integration_id":9007199254740993,"future_child":1e400},{"context":"lint","integration_id":77}],"strict_required_status_checks_policy":false,"future_parameter":-0}',
    ) as Record<string, unknown>;
    const sent = vi.fn();
    render(SyncRulesetPage, {
      ...shared,
      name: 'guard',
      config: config({
        rulesets: [
          { ...savedProtection(), name: 'guard', rules: { required_status_checks: rule } },
        ],
      }),
      onChangeDocument: sent,
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    expect(screen.getByText('App ID 9007199254740993')).toBeTruthy();
    await fireEvent.click(screen.getByRole('button', { name: 'Remove test' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Add a check' }));
    const input = screen.getByRole('textbox', { name: 'Check to add' });
    await fireEvent.input(input, { target: { value: 'test' } });
    await fireEvent.submit(input.closest('form')!);
    await fireEvent.click(
      screen.getByRole('checkbox', { name: 'Require branches to be up to date' }),
    );
    await fireEvent.click(screen.getByRole('button', { name: 'Done' }));
    expect(sent).toHaveBeenCalledTimes(1);
    expect(formatJson(sent.mock.calls[0]![0].rulesets[0].rules.required_status_checks)).toBe(
      formatJson({ ...rule, strict_required_status_checks_policy: true } as JsonValue),
    );
  });

  it('changes a scanning threshold without replacing its tool record or other thresholds', async () => {
    const rule = parseJson(
      '{"code_scanning_tools":[{"tool":"CodeQL","alerts_threshold":"errors","security_alerts_threshold":"critical","future_child":9007199254740993}],"future_parameter":1e-400}',
    ) as Record<string, unknown>;
    const sent = vi.fn();
    render(SyncRulesetPage, {
      ...shared,
      name: 'guard',
      config: config({
        rulesets: [{ ...savedProtection(), name: 'guard', rules: { code_scanning: rule } }],
      }),
      onChangeDocument: sent,
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    await chooseOption(
      screen.getByRole('combobox', { name: 'CodeQL alerts' }),
      'Errors and warnings',
    );
    await fireEvent.click(screen.getByRole('button', { name: 'Done' }));
    const tools = rule.code_scanning_tools as Record<string, unknown>[];
    expect(formatJson(sent.mock.calls[0]![0].rulesets[0].rules.code_scanning)).toBe(
      formatJson({
        ...rule,
        code_scanning_tools: [{ ...tools[0], alerts_threshold: 'errors_and_warnings' }],
      } as JsonValue),
    );
  });

  it('abandons edited values on Cancel and starts again from the current rule', async () => {
    const sent = vi.fn();
    render(SyncRulesetPage, {
      ...shared,
      name: 'main-protection',
      config: config({ rulesets: [savedProtection()] }),
      onChangeDocument: sent,
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    await fireEvent.input(screen.getByRole('spinbutton', { name: 'Approvals required' }), {
      target: { value: '4' },
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(sent).not.toHaveBeenCalled();
    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    expect(
      (screen.getByRole('spinbutton', { name: 'Approvals required' }) as HTMLInputElement).value,
    ).toBe('1');
  });

  it('retains and blocks an existing editor when its rule is removed externally', async () => {
    const sent = vi.fn();
    const rendered = render(SyncRulesetPage, {
      ...shared,
      name: 'main-protection',
      config: config({ rulesets: [savedProtection()] }),
      onChangeDocument: sent,
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    await rendered.rerender({
      config: config({ rulesets: [{ ...savedProtection(), rules: {} }] }),
    });
    expect((screen.getByRole('button', { name: 'Done' }) as HTMLButtonElement).disabled).toBe(true);
    expect(
      screen.getByText(
        'This rule changed elsewhere · close and reopen the editor to review its current values',
      ),
    ).toBeTruthy();
    expect(sent).not.toHaveBeenCalled();
  });
  it('keeps an invalid approval count editable and never stages it', async () => {
    const sent = vi.fn();
    render(SyncRulesetPage, {
      ...shared,
      name: 'main-protection',
      config: config({ rulesets: [savedProtection()] }),
      onChangeDocument: sent,
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    const input = screen.getByRole('spinbutton', { name: 'Approvals required' });
    await fireEvent.input(input, { target: { value: '11' } });
    expect(screen.getByText('Choose a whole number from 0 to 10')).toBeTruthy();
    expect((screen.getByRole('button', { name: 'Done' }) as HTMLButtonElement).disabled).toBe(true);
    await fireEvent.click(screen.getByRole('button', { name: 'Done' }));
    expect(sent).not.toHaveBeenCalled();
    await fireEvent.input(input, { target: { value: '2' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Done' }));
    expect(
      sent.mock.calls[0]![0].rulesets[0].rules.pull_request.required_approving_review_count,
    ).toBe(2);
  });

  it('requires a valid scanning tool before staging a new rule', async () => {
    const sent = vi.fn();
    render(SyncRulesetPage, {
      ...shared,
      name: 'guard',
      config: config({ rulesets: [{ ...savedProtection(), name: 'guard', rules: {} }] }),
      onChangeDocument: sent,
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Add a rule' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Require code scanning' }));
    expect((screen.getByRole('button', { name: 'Done' }) as HTMLButtonElement).disabled).toBe(true);
    expect(sent).not.toHaveBeenCalled();
    await fireEvent.click(screen.getByRole('button', { name: 'Add a tool' }));
    const input = screen.getByRole('textbox', { name: 'Tool to add' });
    await fireEvent.input(input, { target: { value: 'CodeQL' } });
    await fireEvent.submit(input.closest('form')!);
    await fireEvent.click(screen.getByRole('button', { name: 'Done' }));
    expect(sent.mock.calls[0]![0].rulesets[0].rules.code_scanning).toEqual({
      code_scanning_tools: [
        { tool: 'CodeQL', alerts_threshold: 'errors', security_alerts_threshold: 'high_or_higher' },
      ],
    });
  });
  it('resolves pinned app names and avatars without rounding adjacent large IDs', async () => {
    const lookup = vi.fn().mockResolvedValue({
      items: [
        {
          actor_type: 'Integration',
          actor_id: JSON.rawJSON('9007199254740993'),
          name: 'Known build app',
          slug: 'known-build-app',
          avatar_url: '/known-app.png',
        },
      ],
    });
    const rules = {
      required_status_checks: {
        required_status_checks: [
          { context: 'unresolved', integration_id: JSON.rawJSON('9007199254740992') },
          { context: 'resolved', integration_id: JSON.rawJSON('9007199254740993') },
        ],
      },
    };
    const rendered = render(RulesetRuleEditor, {
      scope: 'first',
      rulesetName: 'guard',
      rules,
      disabled: false,
      lookup,
      onApply: vi.fn(),
    });
    rendered.component.show('required_status_checks');
    await tick();
    expect(await screen.findByText('Known build app')).toBeTruthy();
    expect(screen.getByText('App name unavailable')).toBeTruthy();
    expect(screen.getByText('App ID 9007199254740992')).toBeTruthy();
    expect(document.querySelector('img.avatar')?.getAttribute('src')).toBe('/known-app.png');
  });

  it('ignores old name lookups after closing and opening another scope', async () => {
    let completeOld!: (value: BypassActorDirectory) => void;
    const old = new Promise<BypassActorDirectory>((resolve) => {
      completeOld = resolve;
    });
    const identity = {
      actor_type: 'Integration',
      actor_id: 77,
      name: 'Current app',
      slug: 'current-app',
      avatar_url: null,
    };
    const lookup = vi
      .fn()
      .mockReturnValueOnce(old)
      .mockResolvedValueOnce({ items: [identity] });
    const rendered = render(RulesetRuleEditor, {
      scope: 'first',
      rulesetName: 'guard',
      rules: {
        required_status_checks: {
          required_status_checks: [{ context: 'test', integration_id: 77 }],
        },
      },
      disabled: false,
      lookup,
      onApply: vi.fn(),
    });
    rendered.component.show('required_status_checks');
    await tick();
    await fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    await rendered.rerender({ scope: 'second' });
    rendered.component.show('required_status_checks');
    await tick();
    expect(await screen.findByText('Current app')).toBeTruthy();
    completeOld({ items: [{ ...identity, name: 'Stale app' }] });
    await tick();
    await tick();
    expect(screen.queryByText('Stale app')).toBeNull();
    expect(screen.getByText('Current app')).toBeTruthy();
  });
  it.each(['4', '11'])(
    'keeps private approvals %s after refusing incidental dismissal',
    async (value) => {
      const sent = vi.fn();
      render(SyncRulesetPage, {
        ...shared,
        name: 'main-protection',
        config: config({ rulesets: [savedProtection()] }),
        onChangeDocument: sent,
      });
      await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
      const input = screen.getByRole('spinbutton', { name: 'Approvals required' });
      await fireEvent.input(input, { target: { value } });
      await fireEvent.click(screen.getByRole('button', { name: 'Close rule editor' }));
      expect(screen.getByRole('dialog', { name: 'Discard rule changes?' })).toBeTruthy();
      expect(input.isConnected).toBe(true);
      expect(sent).not.toHaveBeenCalled();
      await fireEvent.click(screen.getByRole('button', { name: 'Keep editing' }));
      expect(screen.getByRole('spinbutton', { name: 'Approvals required' })).toBe(input);
      expect((input as HTMLInputElement).value).toBe(value);
      await fireEvent.click(screen.getByRole('button', { name: 'Close rule editor' }));
      await fireEvent.click(screen.getByRole('button', { name: 'Discard changes' }));
      expect(screen.queryByRole('button', { name: 'Done' })).toBeNull();
      expect(sent).not.toHaveBeenCalled();
    },
  );

  it('dismisses an unchanged or exactly reverted private rule without confirmation', async () => {
    render(SyncRulesetPage, {
      ...shared,
      name: 'main-protection',
      config: config({ rulesets: [savedProtection()] }),
    });
    for (const change of [false, true]) {
      await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
      if (change) {
        const input = screen.getByRole('spinbutton', { name: 'Approvals required' });
        await fireEvent.input(input, { target: { value: '4' } });
        await fireEvent.input(input, { target: { value: '1' } });
      }
      await fireEvent.click(screen.getByRole('button', { name: 'Close rule editor' }));
      expect(screen.queryByRole('button', { name: 'Done' })).toBeNull();
      expect(screen.queryByRole('dialog', { name: 'Discard rule changes?' })).toBeNull();
    }
  });

  it('invalidates the confirmation and private draft when its scope changes', async () => {
    const sent = vi.fn();
    const rendered = render(SyncRulesetPage, {
      ...shared,
      name: 'main-protection',
      config: config({ rulesets: [savedProtection()] }),
      onChangeDocument: sent,
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    await fireEvent.input(screen.getByRole('spinbutton', { name: 'Approvals required' }), {
      target: { value: '4' },
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Close rule editor' }));
    expect(screen.getByRole('dialog', { name: 'Discard rule changes?' })).toBeTruthy();
    await rendered.rerender({ name: 'another-ruleset' });
    expect(screen.queryByRole('button', { name: 'Discard changes' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Done' })).toBeNull();
    expect(sent).not.toHaveBeenCalled();
  });
  it.each(['changed', 'removed', 'created'] as const)(
    'blocks stale private rules when the live rule is %s',
    async (change) => {
      const opening = { required_approving_review_count: 1, allowed_merge_methods: ['squash'] };
      const make = (rule: typeof opening | undefined) => ({
        ...savedProtection(),
        rules: rule ? { pull_request: rule } : {},
      });
      const sent = vi.fn();
      const rendered = render(SyncRulesetPage, {
        ...shared,
        name: 'main-protection',
        config: config({ rulesets: [make(change === 'created' ? undefined : opening)] }),
        onChangeDocument: sent,
      });
      if (change === 'created') {
        await fireEvent.click(screen.getByRole('button', { name: 'Add a rule' }));
        await fireEvent.click(screen.getByRole('button', { name: 'Require a pull request' }));
      } else await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
      const input = screen.getByRole('spinbutton', { name: 'Approvals required' });
      await fireEvent.input(input, { target: { value: '4' } });
      await rendered.rerender({
        config: config({
          rulesets: [
            make(
              change === 'removed' ? undefined : { ...opening, required_approving_review_count: 2 },
            ),
          ],
        }),
      });
      expect(
        screen.getByText(
          'This rule changed elsewhere · close and reopen the editor to review its current values',
        ),
      ).toBeTruthy();
      expect(input.isConnected).toBe(true);
      expect((input as HTMLInputElement).value).toBe('4');
      expect((screen.getByRole('button', { name: 'Done' }) as HTMLButtonElement).disabled).toBe(
        true,
      );
      await fireEvent.click(screen.getByRole('button', { name: 'Done' }));
      expect(sent).not.toHaveBeenCalled();
      await rendered.rerender({
        config: config({ rulesets: [make(change === 'created' ? undefined : opening)] }),
      });
      expect(
        screen.queryByText(
          'This rule changed elsewhere · close and reopen the editor to review its current values',
        ),
      ).toBeNull();
      await fireEvent.click(screen.getByRole('button', { name: 'Done' }));
      expect(
        sent.mock.calls[0]![0].rulesets[0].rules.pull_request.required_approving_review_count,
      ).toBe(4);
    },
  );

  it('compares all lossless rule parameters while ignoring unrelated live rule changes', async () => {
    const opening = parseJson(
      '{"required_status_checks":[{"context":"build","integration_id":9007199254740992}],"future":1e400}',
    ) as Record<string, unknown>;
    const changed = parseJson(
      '{"required_status_checks":[{"context":"build","integration_id":9007199254740993}],"future":1e400}',
    ) as Record<string, unknown>;
    const sent = vi.fn();
    const make = (rule: Record<string, unknown>, deletion = true) => ({
      ...savedProtection(),
      rules: { required_status_checks: rule, deletion },
    });
    const rendered = render(SyncRulesetPage, {
      ...shared,
      name: 'main-protection',
      config: config({ rulesets: [make(opening)] }),
      onChangeDocument: sent,
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    await rendered.rerender({ config: config({ rulesets: [make(changed)] }) });
    expect((screen.getByRole('button', { name: 'Done' }) as HTMLButtonElement).disabled).toBe(true);
    await rendered.rerender({
      config: config({ rulesets: [make({ ...opening, future: JSON.rawJSON('1e401') })] }),
    });
    expect((screen.getByRole('button', { name: 'Done' }) as HTMLButtonElement).disabled).toBe(true);
    await rendered.rerender({ config: config({ rulesets: [make(opening, false)] }) });
    expect((screen.getByRole('button', { name: 'Done' }) as HTMLButtonElement).disabled).toBe(
      false,
    );
    await fireEvent.click(screen.getByRole('button', { name: 'Done' }));
    const stored = sent.mock.calls[0]![0].rulesets[0].rules;
    expect(formatJson(stored.required_status_checks)).toBe(formatJson(opening as JsonValue));
    expect(stored.deletion).toBe(false);
  });
  it('keeps the private rule when compare-and-stage rejects an unseen document change', async () => {
    const saved = { rulesets: [savedProtection()], future_document: JSON.rawJSON('1e400') };
    const stage = vi.fn().mockReturnValue(false);
    render(SyncRulesetPage, {
      ...shared,
      name: 'main-protection',
      config: config(saved),
      onChangeDocument: stage,
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    const input = screen.getByRole('spinbutton', { name: 'Approvals required' });
    await fireEvent.input(input, { target: { value: '4' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Done' }));
    expect(stage).toHaveBeenCalledOnce();
    expect(stage.mock.calls[0]![1]).toBe(saved);
    expect(screen.getByRole('spinbutton', { name: 'Approvals required' })).toBe(input);
    expect((input as HTMLInputElement).value).toBe('4');
    expect(
      screen.getByText(
        'This edit could not be staged · close and reopen the editor to review the current settings',
      ),
    ).toBeTruthy();
    expect((screen.getByRole('button', { name: 'Done' }) as HTMLButtonElement).disabled).toBe(true);
    await fireEvent.click(screen.getByRole('button', { name: 'Done' }));
    expect(stage).toHaveBeenCalledOnce();
    await fireEvent.click(screen.getByRole('button', { name: 'Close rule editor' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Keep editing' }));
    expect((input as HTMLInputElement).value).toBe('4');
    await fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    stage.mockReturnValue(true);
    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    expect(
      screen.queryByText(
        'This edit could not be staged · close and reopen the editor to review the current settings',
      ),
    ).toBeNull();
    await fireEvent.input(screen.getByRole('spinbutton', { name: 'Approvals required' }), {
      target: { value: '3' },
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Done' }));
    expect(stage).toHaveBeenCalledTimes(2);
    expect(screen.queryByRole('button', { name: 'Done' })).toBeNull();
  });
});

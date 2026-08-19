// @vitest-environment jsdom
import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import SyncRulesetDetail from '../src/lib/components/SyncRulesetDetail.svelte';
import SyncRulesetsForm from '../src/lib/components/SyncRulesetsForm.svelte';

/** The segmented controls measure themselves to place a thumb; jsdom does not. */
class TestResizeObserver {
  observe(): void {}
  disconnect(): void {}
}

/** The organization's own ruleset, in the shape the document stores it. */
function protection(over: Record<string, unknown> = {}) {
  return {
    name: 'main-branch-protection',
    target: 'branch',
    enforcement: 'active',
    conditions: { include: ['~DEFAULT_BRANCH'], exclude: [] },
    bypass_actors: [{ actor_id: 0, actor_type: 'OrganizationAdmin', bypass_mode: 'always' }],
    rules: {
      deletion: true,
      non_fast_forward: true,
      pull_request: {
        required_approving_review_count: 1,
        require_code_owner_review: true,
        allowed_merge_methods: ['squash'],
      },
      // Configured through the API rather than here. Nothing either page below
      // touches it, so it is what a write has to carry through untouched.
      code_scanning: {
        code_scanning_tools: [
          { tool: 'CodeQL', alerts_threshold: 'errors', security_alerts_threshold: 'all' },
        ],
      },
    },
    ...over,
  };
}

/**
 * A ruleset is written by replacement: the request defines the whole object and
 * what it does not carry stops being enforced. That makes these pages' job
 * narrow and their failure mode wide - anything dropped on the way to a write is
 * something that stops being enforced on every repository, with the plan
 * reporting it as an ordinary change.
 */
describe('SyncRulesetsForm [Component]', () => {
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
    rulesetHref: (name: string) => `/sync/rulesets/${name}`,
    onSave: () => {},
  };

  /** Two levels deep and no deeper: a row is a way into the ruleset's own page. */
  it('draws a row per ruleset, wearing its enforcement and its way in', () => {
    const { container } = render(SyncRulesetsForm, {
      ...base,
      stored: { rulesets: [protection(), protection({ name: 'tags', enforcement: 'evaluate' })] },
    });

    const rows = [...container.querySelectorAll<HTMLAnchorElement>('a.object-row')];
    expect(rows).toHaveLength(2);
    expect(rows[0].getAttribute('href')).toBe('/sync/rulesets/main-branch-protection');
    expect(rows[0].textContent).toContain('Active');
    // Evaluate is a ruleset that looks enforced and enforces nothing, which is
    // exactly the fact a list must not hide behind a press.
    expect(rows[1].textContent).toContain('Evaluate');
  });

  it('says what a ruleset is in one line', () => {
    const { container } = render(SyncRulesetsForm, {
      ...base,
      stored: { rulesets: [protection()] },
    });

    expect(container.querySelector('.object-sum')?.textContent).toBe(
      'default branch · 4 rules · 1 bypass actor',
    );
  });

  /**
   * A ruleset with no action in a computed plan is in step. With no plan, the
   * same ruleset has simply not been looked at, and a check would be the panel
   * answering a question nobody has asked GitHub yet.
   */
  it('marks nothing while no plan has been worked out', () => {
    const { container } = render(SyncRulesetsForm, {
      ...base,
      stored: { rulesets: [protection()] },
    });

    expect(container.querySelector('.state-mark')).toBeNull();
  });

  it('carries what the plan would do about one ruleset', () => {
    const { container } = render(SyncRulesetsForm, {
      ...base,
      stored: { rulesets: [protection()] },
      markOf: () => ({ state: 'change' as const, label: '2 repositories differ' }),
    });

    expect(container.querySelector('.state-mark')?.textContent).toContain('2 repositories differ');
  });

  /** Every write carries the whole document, keys this version knows nothing of included. */
  it('keeps the rest of the document when the removal switch is flipped', async () => {
    const onSave = vi.fn();
    render(SyncRulesetsForm, {
      ...base,
      enabled: true,
      stored: { rulesets: [protection()], allow_removal: false, some_future_key: 'kept' },
      onSave,
    });

    await fireEvent.click(
      screen.getByRole('switch', { name: 'Remove rulesets this list does not name' }),
    );

    expect(onSave.mock.calls[0]?.[0]).toBe(true);
    expect(onSave.mock.calls[0]?.[1]).toMatchObject({
      allow_removal: true,
      some_future_key: 'kept',
    });
  });

  /**
   * The safety valve beside the removal switch, and the reason it is on this
   * page rather than only in the API: somebody who can turn removal on from
   * here has to be able to protect something from here too.
   */
  it('adds a pattern to the rulesets it leaves alone', async () => {
    const onSave = vi.fn();
    render(SyncRulesetsForm, { ...base, stored: { rulesets: [], excludes: [] }, onSave });

    await fireEvent.click(screen.getByRole('button', { name: 'Add a pattern' }));
    const field = screen.getByRole('textbox', { name: 'Add a pattern' });
    await fireEvent.input(field, { target: { value: 'hand-made-*' } });
    await fireEvent.keyDown(field, { key: 'Enter' });

    expect(onSave.mock.calls[0]?.[1]).toMatchObject({ excludes: ['hand-made-*'] });
  });

  /**
   * A new ruleset starts on the default branch of every repository, because
   * refs/heads/main protects nothing on the ones still calling it master - and
   * with no rules, so what it enforces is decided on its own page.
   */
  it('adds a ruleset by name, with no rules and the portable ref', async () => {
    const onSave = vi.fn();
    render(SyncRulesetsForm, { ...base, stored: { rulesets: [] }, onSave });

    await fireEvent.click(screen.getByRole('button', { name: 'Add a ruleset' }));
    const field = screen.getByRole('textbox', { name: 'Name of the ruleset to add' });
    await fireEvent.input(field, { target: { value: 'tag-protection' } });
    await fireEvent.keyDown(field, { key: 'Enter' });

    expect(onSave.mock.calls[0]?.[1]).toEqual({
      rulesets: [
        {
          name: 'tag-protection',
          target: 'branch',
          enforcement: 'active',
          conditions: { include: ['~DEFAULT_BRANCH'], exclude: [] },
          rules: {},
        },
      ],
    });
  });

  it('says what an unreadable document means and changes nothing', () => {
    render(SyncRulesetsForm, { ...base, unreadable: true });

    expect(screen.getByRole('alert').textContent).toContain('cannot read');
    expect(screen.getByRole('switch', { name: 'Syncing' }).hasAttribute('disabled')).toBe(true);
  });

  it('shows a failure of its own beside its own controls', () => {
    render(SyncRulesetsForm, { ...base, problem: 'the rulesets changed; reload' });

    expect(screen.getByRole('alert').textContent).toContain('reload');
  });

  it('says which permission is missing while the switch is on', () => {
    render(SyncRulesetsForm, {
      ...base,
      enabled: true,
      unavailable: 'Smyklot has not been granted administration access, which rulesets need',
    });

    expect(screen.getByRole('status').textContent).toContain('administration');
  });

  it('says nothing of a permission while the switch is off', () => {
    render(SyncRulesetsForm, {
      ...base,
      enabled: false,
      unavailable: 'Smyklot has not been granted administration access, which rulesets need',
    });

    expect(screen.queryByRole('status')).toBeNull();
  });

  it('offers no way to add one while read only', () => {
    render(SyncRulesetsForm, { ...base, stored: { rulesets: [] }, readOnly: true });

    expect(screen.queryByRole('button', { name: 'Add a ruleset' })).toBeNull();
  });
});

describe('SyncRulesetDetail [Component]', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', TestResizeObserver);
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  afterEach(() => vi.unstubAllGlobals());

  const base = {
    stored: { rulesets: [protection()] },
    name: 'main-branch-protection',
    listHref: '/sync/rulesets',
    readOnly: false,
    saving: false,
    unreadable: false,
    onSave: () => {},
  };

  function rowNames(container: HTMLElement): string[] {
    return [...container.querySelectorAll<HTMLElement>('.policy-row .setting-name')].map(
      (name) => name.textContent?.trim() ?? '',
    );
  }

  /** Only what is on is a row; what is off is one sentence naming it. */
  it('draws the rules that are on, and says how many are not', () => {
    const { container } = render(SyncRulesetDetail, base);

    expect(rowNames(container)).toEqual([
      // Where it applies, which is always three rows: a ruleset that covers
      // nothing is still a ruleset with somewhere it would apply.
      'Applies to',
      'Refs it covers',
      'Refs it leaves out',
      'Require a pull request',
      'Block force pushes',
      'Block deletion',
      'Require code scanning',
      // The bypass actor's own row, in the card below the rules.
      'Organization admin',
    ]);
    expect(container.querySelector('.group-tally')?.textContent).toBe('4 of 9 rules on');
    expect(container.querySelector('.rest-count')?.textContent).toBe('5 rules are off');
  });

  /** The parameters are what the row is saying, so they are read without opening it. */
  it("wears a rule's parameters as chips", () => {
    const { container } = render(SyncRulesetDetail, base);

    const params = (container.querySelector('.rule-params')?.textContent ?? '')
      .replace(/\s+/g, ' ')
      .trim();
    expect(params).toContain('1 approval');
    expect(params).toContain('from code owners');
    expect(params).toContain('merged by squash');
  });

  /**
   * GitHub writes a ruleset by replacement, so a rule the document does not
   * carry is a rule that is not enforced - and `false` is a longer way of
   * saying the same thing that a later reader has to work out.
   */
  it('removes a rule rather than storing it as false', async () => {
    const onSave = vi.fn();
    const { container } = render(SyncRulesetDetail, { ...base, onSave });

    const row = [...container.querySelectorAll<HTMLElement>('.policy-row')].find(
      (candidate) =>
        candidate.querySelector('.setting-name')?.textContent?.trim() === 'Block deletion',
    );
    await fireEvent.click(within(row as HTMLElement).getByRole('button'));

    const rules = (onSave.mock.calls[0]?.[0] as { rulesets: { rules: object }[] }).rulesets[0]
      .rules;
    expect(rules).not.toHaveProperty('deletion');
    // And everything else survives, code scanning included - which nothing on
    // this page touched.
    expect(rules).toHaveProperty('code_scanning');
  });

  /** Turning one on gives it the smallest shape GitHub will accept. */
  it('starts a parameterised rule at its smallest legal shape', async () => {
    const onSave = vi.fn();
    render(SyncRulesetDetail, {
      ...base,
      stored: { rulesets: [protection({ rules: {} })] },
      onSave,
    });

    await fireEvent.click(screen.getByRole('button', { name: 'Manage' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Require status checks' }));

    const rules = (onSave.mock.calls[0]?.[0] as { rulesets: { rules: Record<string, unknown> }[] })
      .rulesets[0].rules;
    expect(rules.required_status_checks).toEqual({ required_status_checks: [] });
  });

  /** Three modes, one expensive wrong pick, each carrying the sentence that tells it apart. */
  it('changes enforcement from the cards', async () => {
    const onSave = vi.fn();
    render(SyncRulesetDetail, { ...base, onSave });

    await fireEvent.click(screen.getByRole('radio', { name: /Evaluate/ }));

    expect(
      (onSave.mock.calls[0]?.[0] as { rulesets: { enforcement: string }[] }).rulesets[0]
        .enforcement,
    ).toBe('evaluate');
  });

  /**
   * Every actor but this one is keyed by a number. GitHub answers an
   * organization admin without one at all, and a comparison that read the
   * number never settled.
   */
  it('asks for no id on an organization admin', async () => {
    const { container } = render(SyncRulesetDetail, base);

    const actor = [...container.querySelectorAll<HTMLElement>('.policy-row')].find(
      (candidate) =>
        candidate.querySelector('.setting-name')?.textContent?.trim() === 'Organization admin',
    );
    await fireEvent.click(within(actor as HTMLElement).getByRole('button', { name: 'Edit' }));

    expect(screen.queryByRole('spinbutton', { name: 'Its id on GitHub' })).toBeNull();
  });

  /** An address written down before the ruleset was renamed. */
  it('says so when no ruleset has that name', () => {
    render(SyncRulesetDetail, { ...base, name: 'archived' });

    expect(document.body.textContent).toContain('No ruleset here is called that');
  });

  it('offers no way to delete it while read only', () => {
    render(SyncRulesetDetail, { ...base, readOnly: true });

    expect(screen.queryByRole('button', { name: 'Delete this ruleset' })).toBeNull();
  });
});

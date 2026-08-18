// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import SyncRulesetsForm from '../src/lib/components/SyncRulesetsForm.svelte';

/** The segmented controls measure themselves to place a thumb; jsdom does not. */
class TestResizeObserver {
  observe(): void {}
  disconnect(): void {}
}

/**
 * A ruleset is written by replacement: the request defines the whole object and
 * what it does not carry stops being enforced. That makes this form's job
 * narrow and its failure mode wide - anything it drops on the way to a save is
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
    onSave: () => {},
  };

  /** The organization's own ruleset, in the shape the document stores it. */
  function protection(over: Record<string, unknown> = {}) {
    return {
      name: 'main-branch-protection',
      target: 'branch',
      enforcement: 'active',
      conditions: { include: ['refs/heads/main'], exclude: [] },
      bypass_actors: [{ actor_id: 5, actor_type: 'OrganizationAdmin', bypass_mode: 'always' }],
      rules: {
        deletion: true,
        non_fast_forward: true,
        pull_request: {
          required_approving_review_count: 1,
          require_code_owner_review: true,
          allowed_merge_methods: ['squash'],
        },
        // Configured through the API rather than here: the form can switch
        // code scanning on and off and edit its tools, but nothing in the
        // fixture below touches it, so it is what a save has to carry through
        // untouched.
        code_scanning: {
          code_scanning_tools: [
            {
              tool: 'CodeQL',
              alerts_threshold: 'errors',
              security_alerts_threshold: 'high_or_higher',
            },
          ],
        },
      },
      ...over,
    };
  }

  function stored(over: Record<string, unknown> = {}) {
    return { rulesets: [protection()], allow_removal: false, ...over };
  }

  /** One radio of a segmented control, by the words beside the row it is in. */
  function radio(container: HTMLElement, label: string, value: string): HTMLInputElement {
    const row = [...container.querySelectorAll<HTMLElement>('.ruleset-row')].find(
      (candidate) => candidate.querySelector('.sync-form-label')?.textContent?.trim() === label,
    );
    expect(row, `no row for ${label}`).toBeTruthy();

    const input = [
      ...(row as HTMLElement).querySelectorAll<HTMLInputElement>('input[type=radio]'),
    ].find((candidate) => candidate.value === value);
    expect(input, `no ${value} control for ${label}`).toBeTruthy();

    return input as HTMLInputElement;
  }

  function save(): HTMLElement {
    return screen.getByRole('button', { name: 'Save rulesets' });
  }

  it('shows a stored ruleset as it is', () => {
    const { container } = render(SyncRulesetsForm, { ...base, stored: stored() });

    expect(screen.getByDisplayValue('main-branch-protection')).toBeTruthy();
    expect(screen.getByDisplayValue('refs/heads/main')).toBeTruthy();
    expect(radio(container, 'Applies to', 'branch').checked).toBe(true);
    expect(radio(container, 'Enforcement', 'active').checked).toBe(true);
    expect(radio(container, 'Block deletion', 'on').checked).toBe(true);
    expect(radio(container, 'Require linear history', 'off').checked).toBe(true);
  });

  /**
   * The guard that matters most here. Exclusions have no control on this form
   * and a code-scanning rule configured through the API has only a partial one,
   * so a form that rebuilt the document from its own state would drop both -
   * and dropping a rule from a ruleset is a rule that stops being enforced
   * everywhere, reported in the plan as an ordinary change.
   */
  it('keeps what it has no control for', async () => {
    const onSave = vi.fn();
    const { container } = render(SyncRulesetsForm, {
      ...base,
      stored: stored({ excludes: ['hand-made'] }),
      onSave,
    });

    await fireEvent.click(radio(container, 'Require signed commits', 'on'));
    await fireEvent.click(save());

    const sent = onSave.mock.calls[0]?.[1] as Record<string, unknown>;
    expect(sent.excludes).toEqual(['hand-made']);

    const ruleset = (sent.rulesets as Record<string, unknown>[])[0];
    expect(ruleset.bypass_actors).toEqual([
      { actor_id: 5, actor_type: 'OrganizationAdmin', bypass_mode: 'always' },
    ]);

    const rules = ruleset.rules as Record<string, unknown>;
    expect(rules.required_signatures).toBe(true);
    expect(rules.deletion).toBe(true);
    expect(rules.code_scanning).toEqual({
      code_scanning_tools: [
        {
          tool: 'CodeQL',
          alerts_threshold: 'errors',
          security_alerts_threshold: 'high_or_higher',
        },
      ],
    });
  });

  /**
   * Modelled everywhere else and reachable from nowhere here, which is a rule
   * somebody can read in a plan and not change on the page that produced it.
   */
  it('offers the rule that restricts updates', async () => {
    const onSave = vi.fn();
    const { container } = render(SyncRulesetsForm, { ...base, stored: stored(), onSave });

    await fireEvent.click(radio(container, 'Restrict updates', 'on'));
    await fireEvent.click(radio(container, 'Still allow fetch and merge', 'off'));
    await fireEvent.click(save());

    const ruleset = (onSave.mock.calls[0]?.[1].rulesets as Record<string, unknown>[])[0];
    expect((ruleset.rules as Record<string, unknown>).update).toEqual({
      update_allows_fetch_and_merge: false,
    });
  });

  /**
   * GitHub refuses a pull-request rule that allows no way of merging, so
   * turning the rule on has to arrive somewhere legal rather than at an empty
   * object the save would bounce.
   */
  it('gives a rule turned on the smallest shape GitHub accepts', async () => {
    const onSave = vi.fn();
    const { container } = render(SyncRulesetsForm, {
      ...base,
      stored: { rulesets: [protection({ rules: {} })] },
      onSave,
    });

    await fireEvent.click(radio(container, 'Require a pull request', 'on'));
    await fireEvent.click(save());

    const ruleset = (onSave.mock.calls[0]?.[1].rulesets as Record<string, unknown>[])[0];
    expect((ruleset.rules as Record<string, never>).pull_request).toEqual({
      required_approving_review_count: 1,
      allowed_merge_methods: ['squash'],
    });
  });

  /**
   * A configuration naming refs/heads/main protects nothing on a repository
   * still calling it master, which is the whole problem an organization-wide
   * tool has. The default is the pattern that means the same thing everywhere.
   */
  it('starts a new ruleset on whatever each repository calls its default branch', async () => {
    const onSave = vi.fn();
    render(SyncRulesetsForm, { ...base, stored: { rulesets: [] }, onSave });

    await fireEvent.click(screen.getByRole('button', { name: 'Add a ruleset' }));
    await fireEvent.click(save());

    const ruleset = (onSave.mock.calls[0]?.[1].rulesets as Record<string, unknown>[])[0];
    expect(ruleset.conditions).toEqual({ include: ['~DEFAULT_BRANCH'], exclude: [] });
    expect(ruleset.target).toBe('branch');
    expect(ruleset.enforcement).toBe('active');
  });

  /**
   * The one control here that destroys something. A ruleset dropped from the
   * list goes on enforcing for ever unless this is on, and turning it on
   * removes whatever a repository has that the list does not name.
   */
  it('leaves removal off until somebody asks for it', async () => {
    const onSave = vi.fn();
    const { container } = render(SyncRulesetsForm, { ...base, stored: stored(), onSave });

    expect(radio(container, 'Remove rulesets this list does not name', 'off').checked).toBe(true);

    await fireEvent.click(radio(container, 'Block creation', 'on'));
    await fireEvent.click(save());

    expect(onSave.mock.calls[0]?.[1].allow_removal).toBe(false);
  });

  /**
   * And it is reachable, because it is the one control on this page that
   * destroys something: a ruleset dropped from the list goes on enforcing for
   * ever until somebody turns this on.
   */
  it('offers the removal switch', async () => {
    const onSave = vi.fn();
    const { container } = render(SyncRulesetsForm, { ...base, stored: stored(), onSave });

    await fireEvent.click(radio(container, 'Remove rulesets this list does not name', 'on'));
    await fireEvent.click(save());

    expect(onSave.mock.calls[0]?.[1].allow_removal).toBe(true);
  });

  it('carries the switch that says whether any of this is enforced', async () => {
    const onSave = vi.fn();
    render(SyncRulesetsForm, { ...base, stored: stored(), onSave });

    await fireEvent.click(screen.getByRole('radio', { name: 'Enabled' }));
    await fireEvent.click(save());

    expect(onSave.mock.calls[0]?.[0]).toBe(true);
  });

  it('offers no save while nothing has changed', () => {
    render(SyncRulesetsForm, { ...base, stored: stored() });

    expect(save().hasAttribute('disabled')).toBe(true);
  });

  /**
   * A kind nobody has configured holds an empty document, and what this form
   * would send is three keys with their defaults. Comparing the two directly
   * offers a save the moment the page loads, on a page nobody has touched.
   */
  it('offers no save on a kind nobody has configured', () => {
    render(SyncRulesetsForm, { ...base, stored: {} });

    expect(save().hasAttribute('disabled')).toBe(true);
  });

  /**
   * The safety valve beside the removal switch. A person who can turn removal
   * on from this page has to be able to protect something from it here too,
   * or the only way to keep a hand-made ruleset is to leave removal off
   * entirely.
   */
  it('carries the rulesets somebody asked to be left alone', async () => {
    const onSave = vi.fn();
    render(SyncRulesetsForm, { ...base, stored: stored(), onSave });

    await fireEvent.change(screen.getByLabelText('Rulesets to leave alone'), {
      target: { value: 'hand-made-*\n  \nrelease-freeze' },
    });
    await fireEvent.click(save());

    expect(onSave.mock.calls[0]?.[1].excludes).toEqual(['hand-made-*', 'release-freeze']);
  });

  /**
   * The same rule the settings form follows: an unreadable document shows
   * nothing, which is also what an unconfigured installation looks like, and a
   * save from that form would send the emptiness back.
   */
  it('changes nothing when the stored document could not be read', () => {
    render(SyncRulesetsForm, { ...base, unreadable: true });

    expect(screen.getByRole('alert').textContent).toContain('cannot read');
    expect(save().hasAttribute('disabled')).toBe(true);
  });

  it('shows a failure of its own beside its own controls', () => {
    render(SyncRulesetsForm, { ...base, problem: 'ruleset "main" is listed twice' });

    expect(screen.getByRole('alert').textContent).toContain('listed twice');
  });

  /**
   * Ruleset sync needs the same permission settings sync does, which no
   * installation has granted yet, so this is the ordinary first-use answer.
   */
  it('says which permission is missing while the switch is on', () => {
    render(SyncRulesetsForm, {
      ...base,
      enabled: true,
      unavailable: 'Smyklot has not been granted administration access, which rulesets sync needs',
    });

    expect(screen.getByRole('status').textContent).toContain('administration');
  });

  it('says nothing of a permission while the switch is off', () => {
    render(SyncRulesetsForm, {
      ...base,
      enabled: false,
      unavailable: 'Smyklot has not been granted administration access, which rulesets sync needs',
    });

    expect(screen.queryByRole('status')).toBeNull();
  });
});

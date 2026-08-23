// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import SyncRulesetPage from '../src/lib/components/SyncRulesetPage.svelte';
import SyncRulesetsPage from '../src/lib/components/SyncRulesetsPage.svelte';
import { parseJson } from '../src/lib/merge';
import type { SyncConfig } from '../src/lib/types';

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

  const shared = {
    readOnly: false,
    problem: null,
    sectionHref: () => '#',
    onOpenSection: () => {},
    onChangeDocument: () => {},
  };

  const listShared = {
    ...shared,
    plan: null,
    rulesetHref: (name: string) => `#/${name}`,
    onOpenRuleset: () => {},
    onToggleEnabled: () => {},
  };

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
    const chip = [...document.querySelectorAll<HTMLButtonElement>('.add-chip')].find((held) =>
      (held.textContent ?? '').includes('Require a pull request'),
    );
    await fireEvent.click(chip as HTMLButtonElement);

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
    await fireEvent.click(row?.querySelector('.setting-clear') as HTMLButtonElement);

    const saved = (sent[0]?.rulesets as Array<Record<string, unknown>>)[0];
    expect(saved?.rules).toEqual({ non_fast_forward: true });
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
});

// @vitest-environment jsdom
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { parseJson } from '../src/lib/merge';
import type { BypassActorIdentity, SyncRulesetBypassActor } from '../src/lib/types';
import { chooseOption, optionLabels } from './support/select';
import BypassActorEditor from '../src/lib/components/BypassActorEditor.svelte';
import BypassPolicyEditor from '../src/lib/components/BypassPolicyEditor.svelte';
import type { BypassActorDirectory } from '../src/lib/types';

const APP = { actor_id: 1197525, actor_type: 'Integration', bypass_mode: 'always' };
const IDENTITY = {
  actor_id: APP.actor_id,
  actor_type: 'Integration',
  name: 'Smyklot',
  slug: 'smyklot',
  avatar_url: 'https://example.test/app.png',
};

describe('shared bypass editors [Component]', () => {
  it('keeps unresolved actors distinct and recovers names without changing permissions', async () => {
    const actors = [APP, { ...APP, actor_id: 2740 }];
    const onChange = vi.fn();
    const lookup = vi.fn(async (): Promise<BypassActorDirectory> => ({ items: [] }));
    render(BypassActorEditor, { actors, lookup, onChange });
    await screen.findByRole('button', { name: 'Retry names' });
    expect(screen.getAllByText('Unavailable app')).toHaveLength(2);
    expect(screen.getByText(/App ID 1197525/)).toBeTruthy();
    const unresolved = screen.getByRole('combobox', {
      name: 'Bypass mode for Unavailable app, App ID 2740',
    });
    await chooseOption(unresolved, 'Pull requests only');
    expect(onChange).toHaveBeenLastCalledWith([APP, { ...actors[1], bypass_mode: 'pull_request' }]);
    lookup.mockResolvedValue({ items: [IDENTITY] });
    await fireEvent.click(screen.getByRole('button', { name: 'Retry names' }));
    await screen.findByRole('button', { name: 'Remove Smyklot' });
    expect(document.querySelector('img')?.getAttribute('src')).toBe(IDENTITY.avatar_url);
    expect(screen.queryByText(/App ID 1197525/)).toBeNull();
    await fireEvent.click(
      screen.getByRole('button', { name: 'Remove Unavailable app, App ID 2740' }),
    );
    expect(onChange).toHaveBeenLastCalledWith([APP]);
    expect(onChange).toHaveBeenCalledTimes(2);
  });

  it('renders lossless active actor IDs and prevents duplicate role additions', async () => {
    const actors = parseJson(
      '[{"actor_type":"Integration","actor_id":1197525,"bypass_mode":"always"},{"actor_type":"RepositoryRole","actor_id":5,"bypass_mode":"always"},{"actor_type":"RepositoryRole","actor_id":901,"bypass_mode":"always"},{"actor_type":"OrganizationAdmin","actor_id":0,"bypass_mode":"always"}]',
    ) as unknown as SyncRulesetBypassActor[];
    const onChange = vi.fn();
    render(BypassActorEditor, { actors, onChange });
    expect(screen.getByRole('button', { name: 'Remove Repository admin' })).toBeTruthy();
    await fireEvent.click(screen.getByRole('button', { name: 'Add an actor' }));
    await chooseOption(screen.getByLabelText('Who'), 'Repository role');
    const add = screen.getByRole<HTMLButtonElement>('button', { name: 'Add actor' });
    expect(add.disabled).toBe(true);
    await chooseOption(screen.getByLabelText('Role'), 'Custom repository role');
    await fireEvent.input(screen.getByLabelText('Custom repository role ID'), {
      target: { value: '901' },
    });
    expect(add.disabled).toBe(true);
    await fireEvent.input(screen.getByLabelText('Custom repository role ID'), {
      target: { value: '902' },
    });
    expect(add.disabled).toBe(false);
    await chooseOption(screen.getByLabelText('Who'), 'Organization admin');
    expect(add.disabled).toBe(true);
    expect(onChange).not.toHaveBeenCalled();
  });

  it('edits and removes one large actor ID without affecting its neighboring integer', async () => {
    const actors = parseJson(
      '[{"actor_type":"Integration","actor_id":9007199254740992,"bypass_mode":"always"},{"actor_type":"Integration","actor_id":9007199254740993,"bypass_mode":"always"}]',
    ) as unknown as SyncRulesetBypassActor[];
    const onChange = vi.fn();
    render(BypassActorEditor, { actors, onChange });
    await chooseOption(
      screen.getByLabelText('Bypass mode for Unavailable app, App ID 9007199254740993'),
      'Pull requests only',
    );
    expect(JSON.stringify(onChange.mock.lastCall?.[0])).toBe(
      '[{"actor_type":"Integration","actor_id":9007199254740992,"bypass_mode":"always"},{"actor_type":"Integration","actor_id":9007199254740993,"bypass_mode":"pull_request"}]',
    );
    await fireEvent.click(
      screen.getByRole('button', { name: 'Remove Unavailable app, App ID 9007199254740992' }),
    );
    expect(JSON.stringify(onChange.mock.lastCall?.[0])).toBe(
      '[{"actor_type":"Integration","actor_id":9007199254740993,"bypass_mode":"always"}]',
    );
  });

  it('adds a lossless directory identity by name without rounding its ID', async () => {
    const identity = parseJson(
      '{"actor_type":"Integration","actor_id":9007199254740993,"name":"Release app","slug":"release-app","avatar_url":null}',
    ) as unknown as BypassActorIdentity;
    const onChange = vi.fn();
    render(BypassActorEditor, {
      actors: [],
      lookup: async () => ({ items: [identity] }),
      onChange,
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Add an actor' }));
    await fireEvent.click(await screen.findByRole('button', { name: 'Add Release app' }));
    expect(JSON.stringify(onChange.mock.lastCall?.[0])).toBe(
      '[{"actor_type":"Integration","actor_id":9007199254740993,"bypass_mode":"always"}]',
    );
  });

  it('resolves a saved app name and avatar and edits its mode without replacing identity', async () => {
    const onChange = vi.fn();
    render(BypassActorEditor, {
      actors: [APP],
      lookup: async () => ({ items: [IDENTITY] }),
      onChange,
    });
    const mode = await screen.findByLabelText('Bypass mode for Smyklot');
    expect(document.querySelector('img')?.getAttribute('src')).toBe(IDENTITY.avatar_url);
    expect(document.body.textContent).not.toContain('App 1197525');
    await chooseOption(mode, 'Pull requests only');
    expect(onChange).toHaveBeenLastCalledWith([{ ...APP, bypass_mode: 'pull_request' }]);
    await fireEvent.click(screen.getByRole('button', { name: 'Remove Smyklot' }));
    expect(onChange).toHaveBeenLastCalledWith([]);
  });

  it('compares row markers with losslessly parsed saved actor identities', async () => {
    const savedActors = parseJson(
      '[{"actor_type":"Integration","actor_id":1197525,"bypass_mode":"always"},{"actor_type":"OrganizationAdmin","actor_id":0,"bypass_mode":"always"}]',
    ) as unknown as SyncRulesetBypassActor[];
    const props = {
      actors: [
        { ...APP, bypass_mode: 'pull_request' },
        { actor_type: 'OrganizationAdmin', actor_id: 0, bypass_mode: 'always' },
      ],
      savedActors,
      onChange: vi.fn(),
    };
    const view = render(BypassActorEditor, props);
    expect(document.querySelectorAll('.actor-row.is-unsaved')).toHaveLength(1);
    await view.rerender({ ...props, actors: [APP, props.actors[1]] });
    expect(document.querySelectorAll('.actor-row.is-unsaved')).toHaveLength(0);
  });

  it('removes existing actors from suggestions and restores them when removed from the live list', async () => {
    const props = { actors: [], lookup: async () => ({ items: [IDENTITY] }), onChange: vi.fn() };
    const view = render(BypassActorEditor, props);
    await fireEvent.click(screen.getByRole('button', { name: 'Add an actor' }));
    await screen.findByRole('button', { name: 'Add Smyklot' });
    await view.rerender({ ...props, actors: [APP] });
    expect(screen.queryByRole('button', { name: 'Add Smyklot' })).toBeNull();
    expect(screen.getByText('Matching actors are already added')).toBeTruthy();
    await view.rerender(props);
    expect(screen.getByRole('button', { name: 'Add Smyklot' })).toBeTruthy();
  });

  it('looks up a named app and saves only its stable identity', async () => {
    const onChange = vi.fn();
    const lookup = vi.fn(async (type?: string): Promise<BypassActorDirectory> => ({
      items: type ? [IDENTITY] : [],
    }));
    render(BypassActorEditor, { actors: [], lookup, onChange });
    await fireEvent.click(screen.getByRole('button', { name: 'Add an actor' }));
    await fireEvent.input(screen.getByLabelText('App name or slug'), {
      target: { value: 'smyklot' },
    });
    await waitFor(() => expect(lookup).toHaveBeenLastCalledWith('Integration', 'smyklot'));
    await fireEvent.click(await screen.findByRole('button', { name: 'Add Smyklot' }));
    expect(lookup).toHaveBeenLastCalledWith('Integration', 'smyklot');
    expect(onChange).toHaveBeenCalledWith([APP]);
  });

  it('debounces typing and filters known actor names immediately', async () => {
    const lookup = vi.fn(async (): Promise<BypassActorDirectory> => ({ items: [IDENTITY] }));
    render(BypassActorEditor, { actors: [], lookup, onChange: vi.fn() });
    await waitFor(() => expect(lookup).toHaveBeenCalledTimes(1));
    await fireEvent.click(screen.getByRole('button', { name: 'Add an actor' }));
    const input = screen.getByLabelText('App name or slug');
    await fireEvent.input(input, { target: { value: 'sm' } });
    await fireEvent.input(input, { target: { value: 'smyk' } });
    await fireEvent.input(input, { target: { value: 'smyklot' } });
    expect(lookup).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('button', { name: 'Add Smyklot' })).toBeTruthy();
    await waitFor(() => expect(lookup).toHaveBeenCalledTimes(2));
    expect(lookup).toHaveBeenLastCalledWith('Integration', 'smyklot');
    expect(screen.queryByRole('button', { name: 'Find actor' })).toBeNull();
  });

  it.each([
    ['not_installed', 'Not installed'],
    ['suspended', 'Installation suspended'],
    ['selected_repositories', 'Repository access unverified'],
    ['unknown', 'Installation unverified'],
  ] as const)('shows %s without silently removing a configured app', async (status, label) => {
    const onChange = vi.fn();
    render(BypassActorEditor, {
      actors: [APP],
      lookup: async () => ({ items: [{ ...IDENTITY, installation_status: status }] }),
      onChange,
    });
    expect(await screen.findByText(label)).toBeTruthy();
    if (status !== 'unknown') expect(screen.getByText(/on GitHub/)).toBeTruthy();
    expect(screen.queryByText(/A bypass does not install an app/)).toBeNull();
    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Remove Smyklot' })).toBeTruthy();
  });

  it('does not apply a late lookup response after switching actor type', async () => {
    let finish: (value: BypassActorDirectory) => void = () => {};
    const lookup = vi.fn((type?: string): Promise<BypassActorDirectory> =>
      type ? new Promise((resolve) => (finish = resolve)) : Promise.resolve({ items: [] }),
    );
    render(BypassActorEditor, { actors: [], lookup, onChange: vi.fn() });
    await fireEvent.click(screen.getByRole('button', { name: 'Add an actor' }));
    await fireEvent.input(screen.getByLabelText('App name or slug'), {
      target: { value: 'smyklot' },
    });
    await waitFor(() => expect(lookup).toHaveBeenLastCalledWith('Integration', 'smyklot'));
    await chooseOption(screen.getByLabelText('Who'), 'Team');
    finish({ items: [IDENTITY] });
    await waitFor(() => expect(screen.queryByText('Smyklot')).toBeNull());
    expect(screen.getByLabelText('Team name or slug')).toBeTruthy();
  });

  it('keeps a saved actor available when lookup fails and prevents invalid deploy-key mode', async () => {
    render(BypassActorEditor, {
      actors: [APP],
      lookup: async () => {
        throw new Error('offline');
      },
      onChange: vi.fn(),
    });
    expect(await screen.findByRole('button', { name: 'Retry names' })).toBeTruthy();
    expect(screen.getByText('Unavailable app')).toBeTruthy();
    await fireEvent.click(screen.getByRole('button', { name: 'Add an actor' }));
    await chooseOption(screen.getByLabelText('Who'), 'Deploy keys');
    expect(await optionLabels(screen.getByLabelText('Permission'))).toEqual([
      'Always allow',
      'Exempt from rules',
    ]);
  });

  it('adds a custom repository role only with a valid ID', async () => {
    const onChange = vi.fn();
    render(BypassActorEditor, { actors: [], onChange });
    await fireEvent.click(screen.getByRole('button', { name: 'Add an actor' }));
    await chooseOption(screen.getByLabelText('Who'), 'Repository role');
    await chooseOption(screen.getByLabelText('Role'), 'Custom repository role');
    const field = screen.getByLabelText('Custom repository role ID');
    await fireEvent.input(field, { target: { value: '4.2' } });
    expect(screen.getByRole<HTMLButtonElement>('button', { name: 'Add actor' }).disabled).toBe(
      true,
    );
    await fireEvent.input(field, { target: { value: '42' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Add actor' }));
    expect(onChange).toHaveBeenCalledWith([
      { actor_id: 42, actor_type: 'RepositoryRole', bypass_mode: 'always' },
    ]);
  });

  it('authors a custom role up to the int64 limit without a float conversion', async () => {
    const onChange = vi.fn();
    render(BypassActorEditor, { actors: [], onChange });
    await fireEvent.click(screen.getByRole('button', { name: 'Add an actor' }));
    await chooseOption(screen.getByLabelText('Who'), 'Repository role');
    await chooseOption(screen.getByLabelText('Role'), 'Custom repository role');
    const field = screen.getByLabelText('Custom repository role ID');
    const add = screen.getByRole<HTMLButtonElement>('button', { name: 'Add actor' });
    for (const value of ['0', '-1', '1.5', '1e3', '9223372036854775808']) {
      await fireEvent.input(field, { target: { value } });
      expect(add.disabled).toBe(true);
    }
    await fireEvent.input(field, { target: { value: '9223372036854775807' } });
    expect(add.disabled).toBe(false);
    await fireEvent.click(add);
    expect(JSON.stringify(onChange.mock.lastCall?.[0])).toBe(
      '[{"actor_type":"RepositoryRole","actor_id":9223372036854775807,"bypass_mode":"always"}]',
    );
  });

  it('does not offer organization actors in a personal workspace but keeps old rows removable', async () => {
    render(BypassActorEditor, {
      actors: [{ actor_type: 'OrganizationAdmin', actor_id: 0, bypass_mode: 'always' }],
      organizationActors: false,
      onChange: vi.fn(),
    });
    expect(screen.getByRole('button', { name: 'Remove Organization admin' })).toBeTruthy();
    await fireEvent.click(screen.getByRole('button', { name: 'Add an actor' }));
    const choices = await optionLabels(screen.getByLabelText('Who'));
    expect(choices).not.toContain('Organization admin');
    expect(choices).not.toContain('Team');
    expect(choices).toContain('App');
  });

  it('keeps verified app rows compact with spaced metadata and no generic warnings', async () => {
    render(BypassActorEditor, {
      actors: [APP],
      lookup: async () => ({ items: [{ ...IDENTITY, installation_status: 'all_repositories' }] }),
      onChange: vi.fn(),
    });
    await screen.findByText('All repositories');
    const row = screen.getByRole('button', { name: 'Remove Smyklot' }).closest('.actor-row');
    expect(row?.textContent?.replace(/\s/gu, ' ')).toContain('App · smyklot · All repositories');
    expect(screen.queryByText(/A bypass does not install/)).toBeNull();
    expect(screen.queryByText(/Always allow supports release/)).toBeNull();
  });

  it('retains actors when exceptions are disabled', async () => {
    const onChange = vi.fn();
    render(BypassPolicyEditor, { value: { allow: true, actors: [APP] }, onChange });
    await chooseOption(screen.getByLabelText('Bypass exception policy'), 'No exceptions');
    expect(onChange).toHaveBeenCalledWith({ allow: false, actors: [APP] });
  });

  it('shows inherited actors read only until this repository selects its own policy', async () => {
    const onChange = vi.fn();
    render(BypassPolicyEditor, {
      value: null,
      inherited: { allow: true, actors: [APP] },
      lookup: async () => ({ items: [IDENTITY] }),
      onChange,
    });
    expect((await screen.findByLabelText('Bypass mode for Smyklot')).textContent).toBe(
      'Always allow',
    );
    expect(screen.queryByRole('combobox', { name: 'Bypass mode for Smyklot' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Add an actor' })).toBeNull();
    await chooseOption(screen.getByLabelText('Bypass exception policy'), 'Allow selected actors');
    expect(onChange).toHaveBeenCalledWith({ allow: true, actors: [APP] });
  });
});

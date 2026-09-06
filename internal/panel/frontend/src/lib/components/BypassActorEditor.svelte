<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { useDebounce } from 'runed';
  import {
    BYPASS_ACTOR_TYPES,
    BYPASS_MODES,
    BYPASS_ROLES,
    bypassActorId,
    bypassActorIdFromText,
    bypassActorKey,
    bypassActorName,
    bypassActorReference,
    bypassInstallationLabel,
  } from '../bypass-policy';
  import type { BypassActorIdentity, BypassActorLookup, SyncRulesetBypassActor } from '../types';
  import Avatar from './Avatar.svelte';
  import Button from './Button.svelte';
  import FormError from './FormError.svelte';
  import Icon from './Icon.svelte';
  import IconButton from './IconButton.svelte';
  import Select from './Select.svelte';

  let {
    actors,
    savedActors,
    showAddButton = true,
    adding = $bindable(false),
    lookup,
    readOnly = false,
    organizationActors = true,
    onChange,
  }: {
    actors: SyncRulesetBypassActor[];
    savedActors?: readonly SyncRulesetBypassActor[];
    showAddButton?: boolean;
    adding?: boolean;
    lookup?: BypassActorLookup;
    readOnly?: boolean;
    organizationActors?: boolean;
    onChange: (actors: SyncRulesetBypassActor[]) => void;
  } = $props();

  let identities = $state<BypassActorIdentity[]>([]);
  let results = $state<BypassActorIdentity[]>([]);
  let warning = $state<string | null>(null);
  let failure = $state<string | null>(null);
  let loading = $state(false);
  let loadingNames = $state(false);
  let namesLoaded = $state(false);
  const namesMessage = $derived(
    warning ??
      (namesLoaded && actors.some((actor) => bypassActorReference(actor, identities))
        ? 'Some actor names are unavailable'
        : null),
  );
  let returnFocus: HTMLElement | undefined;
  let focusGeneration = 0;
  let formElement = $state<HTMLFormElement | null>(null);

  export function openAdd(trigger?: HTMLElement): void {
    if (readOnly) return;
    returnFocus = trigger;
    adding = true;
    const request = ++focusGeneration;
    void tick().then(() => {
      if (request !== focusGeneration || !adding || !formElement?.isConnected) return;
      const field = formElement?.querySelector<HTMLElement>('[role="combobox"]');
      field?.focus({ preventScroll: true });
      if (field) revealControl(field);
    });
  }

  export function toggleAdd(trigger?: HTMLElement): void {
    if (adding) closeAdd();
    else openAdd(trigger);
  }

  function closeAdd(): void {
    adding = false;
    const request = ++focusGeneration;
    generation++;
    loading = false;
    returnFocus?.focus({ preventScroll: true });
    void tick().then(() => {
      if (request === focusGeneration && !adding && returnFocus?.isConnected)
        revealControl(returnFocus);
    });
  }

  function revealControl(control: HTMLElement): void {
    const bounds = control.getBoundingClientRect();
    if (bounds.width === 0 || bounds.height === 0) return;
    const composer = document.querySelector('.settings-composer')?.getBoundingClientRect();
    const lowerEdge =
      Math.min(
        window.innerHeight,
        composer && composer.top > 0 && composer.top < window.innerHeight && composer.height > 0
          ? composer.top
          : window.innerHeight,
      ) - 8;
    const upperEdge =
      Math.max(0, document.querySelector('.top-bar')?.getBoundingClientRect().bottom ?? 0) + 8;
    const delta =
      bounds.bottom > lowerEdge
        ? bounds.bottom - lowerEdge
        : bounds.top < upperEdge
          ? bounds.top - upperEdge
          : 0;
    if (delta) window.scrollBy({ top: delta, behavior: 'instant' });
  }

  function actorChanged(actor: SyncRulesetBypassActor): boolean {
    if (!savedActors) return false;
    const saved = savedActors.find((item) => bypassActorKey(item) === bypassActorKey(actor));
    return !saved || saved.bypass_mode !== actor.bypass_mode;
  }
  let actorType = $state('Integration');
  let query = $state('');
  let searched = $state(false);
  let role = $state<string | number>(5);
  let customRoleId = $state('');
  const roleId = $derived(bypassActorIdFromText(role === 'custom' ? customRoleId : String(role)));
  const validRole = $derived(roleId !== undefined && roleId !== 0);
  const duplicateRole = $derived(
    actors.some(
      (actor) =>
        actor.actor_type === actorType &&
        bypassActorId(actor) ===
          bypassActorId({ actor_id: actorType === 'RepositoryRole' ? (roleId ?? -1) : 0 }),
    ),
  );
  const roleOptions = [...BYPASS_ROLES, { value: 'custom', label: 'Custom repository role' }];
  let mode = $state('always');
  let generation = 0;
  const actorTypes = $derived(
    BYPASS_ACTOR_TYPES.filter(
      (type) => organizationActors || !['OrganizationAdmin', 'Team'].includes(type.value),
    ),
  );
  const named = $derived(['Integration', 'Team', 'User'].includes(actorType));
  const choices = $derived(
    BYPASS_MODES.filter((item) => actorType !== 'DeployKey' || item.value !== 'pull_request'),
  );

  const candidates = $derived.by(() => {
    const needle = query.trim().toLocaleLowerCase();
    const local = identities.filter(
      (identity) =>
        identity.actor_type === actorType &&
        (!needle || `${identity.name} ${identity.slug}`.toLocaleLowerCase().includes(needle)),
    );
    return [
      ...local.filter(
        (known) => !results.some((result) => bypassActorKey(result) === bypassActorKey(known)),
      ),
      ...results,
    ];
  });
  const suggestions = $derived(
    candidates.filter(
      (candidate) => !actors.some((actor) => bypassActorKey(actor) === bypassActorKey(candidate)),
    ),
  );
  const allMatchesAdded = $derived(candidates.length > 0 && suggestions.length === 0);
  const debouncedSearch = useDebounce(
    (request: number, type: string, value: string) => void search(request, type, value),
    250,
  );

  function typeQuery(value: string): void {
    query = value;
    results = [];
    searched = false;
    failure = null;
    const request = ++generation;
    loading = Boolean(lookup && value.trim().length >= 1);
    if (loading) void debouncedSearch(request, actorType, value.trim());
  }

  onMount(() => {
    void loadIdentities();
    return () => {
      generation++;
    };
  });

  function remember(items: BypassActorIdentity[]): void {
    identities = [
      ...identities.filter(
        (known) => !items.some((item) => bypassActorKey(known) === bypassActorKey(item)),
      ),
      ...items,
    ];
  }

  async function loadIdentities(): Promise<void> {
    if (!lookup || loadingNames) return;
    loadingNames = true;
    try {
      const response = await lookup();
      remember(response.items);
      warning = response.warning?.replace(/\.+$/u, '') ?? null;
    } catch {
      warning = 'Names unavailable, saved exceptions are still editable';
    } finally {
      loadingNames = false;
      namesLoaded = true;
    }
  }

  function changeType(value: string): void {
    actorType = value;
    results = [];
    query = '';
    searched = false;
    failure = null;
    generation++;
    loading = false;
    if (value === 'DeployKey' && mode === 'pull_request') mode = 'always';
  }

  async function search(request: number, type: string, value: string): Promise<void> {
    if (
      !lookup ||
      request !== generation ||
      !adding ||
      actorType !== type ||
      query.trim() !== value
    )
      return;
    try {
      const response = await lookup(type, value);
      if (request !== generation) return;
      remember(response.items);
      results = response.items;
      warning = response.warning?.replace(/\.+$/u, '') ?? null;
      searched = true;
    } catch (error) {
      if (request === generation)
        failure =
          error instanceof Error ? error.message.replace(/\.+$/u, '') : 'Could not find this actor';
    } finally {
      if (request === generation) loading = false;
    }
  }

  function add(identity?: BypassActorIdentity): void {
    if (readOnly || (!identity && actorType === 'RepositoryRole' && !validRole)) return;
    const actor = {
      actor_type: identity?.actor_type ?? actorType,
      actor_id: identity?.actor_id ?? (actorType === 'RepositoryRole' ? (roleId ?? 0) : 0),
      bypass_mode: mode,
    };
    if (actors.some((held) => bypassActorKey(held) === bypassActorKey(actor))) return;
    onChange([...actors, actor]);
    closeAdd();
    results = [];
    query = '';
    searched = false;
  }

  function installationHelp(identity: BypassActorIdentity | undefined): string | null {
    switch (identity?.installation_status) {
      case 'not_installed':
        return 'Install this app on GitHub before it can bypass';
      case 'suspended':
        return 'Resume its installation on GitHub';
      case 'selected_repositories':
        return 'Check repository access on GitHub';
      default:
        return null;
    }
  }

  const searchStatus = $derived(
    named && !lookup
      ? 'GitHub lookup is unavailable'
      : !named && duplicateRole
        ? 'This actor is already listed'
        : named && allMatchesAdded
          ? 'Matching actors are already added'
          : loading
            ? 'Looking for actors'
            : searched && suggestions.length === 0
              ? 'No matches, try an exact username or slug'
              : null,
  );

  function updateMode(actor: SyncRulesetBypassActor, value: string): void {
    if (readOnly) return;
    onChange(
      actors.map((held) =>
        bypassActorKey(held) === bypassActorKey(actor) ? { ...held, bypass_mode: value } : held,
      ),
    );
  }
</script>

<!--
@component
The shared actor list for managed rulesets and merge exceptions. Identity lookup
supplies names and pictures for display; only stable GitHub IDs and bypass modes
leave through onChange. Existing actors remain editable when lookup is unavailable.
The add form stages an actor only after a named result is chosen. Read-only mode
also supports inherited lists without turning them into local overrides.
-->

{#if !readOnly && showAddButton}
  <div class="actor-header">
    <Button tone="quiet" aria-expanded={adding} onclick={(event) => toggleAdd(event.currentTarget)}>
      {#snippet icon()}<Icon name="plus" size="sm" />{/snippet}Add an actor
    </Button>
  </div>
{/if}

{#if actors.length > 0}
  <ul
    class="object-list actor-list"
    class:actor-list-continues={adding || Boolean(namesMessage)}
    aria-label="Bypass exceptions"
  >
    {#each actors as actor (bypassActorKey(actor))}
      {@const identity = identities.find((item) => bypassActorKey(item) === bypassActorKey(actor))}
      {@const name = bypassActorName(actor, identities)}
      {@const reference = bypassActorReference(actor, identities)}
      {@const controlName = reference ? `${name}, ${reference}` : name}
      <li>
        <div
          class="object-row actor-row"
          class:is-unsaved={actorChanged(actor)}
          data-unsaved={actorChanged(actor) || undefined}
        >
          <div class="actor-identity">
            {#if ['Integration', 'Team', 'User'].includes(actor.actor_type)}
              <Avatar
                account={{
                  id: bypassActorId(actor),
                  provider: 'github',
                  subject_id: bypassActorId(actor),
                  login: identity?.slug ?? name,
                  display_name: name,
                  avatar_url: identity?.avatar_url ?? null,
                }}
                size={32}
                shape={actor.actor_type === 'User' ? 'person' : 'workspace'}
              />
            {:else}
              <span class="actor-symbol" aria-hidden="true"
                ><Icon
                  name={actor.actor_type === 'DeployKey'
                    ? 'lock'
                    : actor.actor_type === 'OrganizationAdmin'
                      ? 'organization'
                      : 'admin'}
                  size="base"
                /></span
              >
            {/if}
            <span class="setting-say">
              <span class="setting-name">{name}</span>
              <span class="setting-why"
                >{reference ??
                  BYPASS_ACTOR_TYPES.find((item) => item.value === actor.actor_type)
                    ?.label}{#if identity?.slug && identity.slug !== name}
                  &nbsp;·&nbsp;{identity.slug}{/if}{#if actor.actor_type === 'Integration'}
                  &nbsp;·&nbsp;<span
                    class:availability-warning={[
                      'selected_repositories',
                      'not_installed',
                      'suspended',
                    ].includes(identity?.installation_status ?? '')}
                    >{bypassInstallationLabel(identity)}</span
                  >{/if}</span
              >
              {#if actor.actor_type === 'Integration' && installationHelp(identity)}
                <span class="setting-why">{installationHelp(identity)}</span>
              {/if}
            </span>
          </div>
          <div class="actor-actions">
            {#if readOnly}
              <span class="setting-unmanaged" aria-label="Bypass mode for {controlName}"
                >{BYPASS_MODES.find((mode) => mode.value === actor.bypass_mode)?.label ??
                  actor.bypass_mode}</span
              >
            {:else}
              <Select
                aria-label="Bypass mode for {controlName}"
                value={actor.bypass_mode}
                options={BYPASS_MODES.filter(
                  (item) => actor.actor_type !== 'DeployKey' || item.value !== 'pull_request',
                )}
                disabled={readOnly}
                onValueChange={(value) => updateMode(actor, value)}
              />
            {/if}
            {#if !readOnly}
              <IconButton
                icon="close"
                label="Remove {controlName}"
                toolbar
                onclick={() =>
                  onChange(actors.filter((held) => bypassActorKey(held) !== bypassActorKey(actor)))}
              />
            {/if}
          </div>
        </div>
      </li>
    {/each}
  </ul>
{:else if readOnly}
  <p class="empty-actors">No actors selected</p>
{/if}

{#if namesMessage}
  <div class="directory-warning" role="status">
    <span class="setting-why">{namesMessage}</span>
    {#if lookup}
      <Button tone="quiet" disabled={loadingNames} onclick={() => void loadIdentities()}
        >Retry names</Button
      >
    {/if}
  </div>
{/if}

{#if !readOnly}
  {#if adding}
    <form
      class="actor-form"
      bind:this={formElement}
      onsubmit={(event) => {
        event.preventDefault();
        if (!named) add();
      }}
    >
      <div class="actor-fields">
        <label class="actor-field"
          ><span class="setting-name">Who</span><Select
            aria-label="Who"
            value={actorType}
            options={actorTypes}
            onValueChange={changeType}
          /></label
        >
        {#if named}
          <label class="actor-field identity-field"
            ><span class="setting-name"
              >{actorType === 'Integration'
                ? 'App name or slug'
                : actorType === 'Team'
                  ? 'Team name or slug'
                  : 'GitHub username'}</span
            ><input
              class="text-input"
              value={query}
              oninput={(event) => typeQuery(event.currentTarget.value)}
              placeholder={actorType === 'Integration'
                ? 'smyklot'
                : actorType === 'Team'
                  ? 'release-engineering'
                  : 'octocat'}
              spellcheck="false"
              autocomplete="off"
            /></label
          >
        {:else if actorType === 'RepositoryRole'}
          <label class="actor-field"
            ><span class="setting-name">Role</span><Select
              aria-label="Role"
              value={role}
              options={roleOptions}
              onValueChange={(value) => (role = value)}
            /></label
          >
        {/if}
        {#if actorType === 'RepositoryRole' && role === 'custom'}
          <label class="actor-field identity-field"
            ><span class="setting-name">Custom repository role ID</span><input
              class="text-input"
              inputmode="numeric"
              bind:value={customRoleId}
              placeholder="Role ID from GitHub"
            /></label
          >
        {/if}
        <label class="actor-field"
          ><span class="setting-name">Permission</span><Select
            aria-label="Permission"
            bind:value={mode}
            options={choices}
          /></label
        >
      </div>
      {#if named}
        {#if suggestions.length > 0}
          <ul class="object-list actor-list-continues" aria-label="Matching actors">
            {#each suggestions as identity (bypassActorKey(identity))}
              <li>
                <button
                  class="object-row result-row"
                  aria-label={`Add ${identity.name}`}
                  type="button"
                  onclick={() => add(identity)}
                  ><Avatar
                    account={{
                      id: bypassActorId(identity),
                      provider: 'github',
                      subject_id: bypassActorId(identity),
                      login: identity.slug,
                      display_name: identity.name,
                      avatar_url: identity.avatar_url,
                    }}
                    size={32}
                    shape={identity.actor_type === 'User' ? 'person' : 'workspace'}
                  /><span class="setting-say"
                    ><span class="setting-name">{identity.name}</span><span class="setting-why"
                      >{identity.slug}{#if identity.actor_type === 'Integration'}
                        &nbsp;·&nbsp;<span
                          class:availability-warning={[
                            'selected_repositories',
                            'not_installed',
                            'suspended',
                          ].includes(identity.installation_status ?? '')}
                          >{bypassInstallationLabel(identity)}</span
                        >{/if}</span
                    ></span
                  ><span class="result-action">Add</span></button
                >
              </li>
            {/each}
          </ul>
        {/if}
      {/if}
      <div class="actor-form-foot">
        <div class="actor-form-status">
          {#if searchStatus}<span class="setting-why" role="status">{searchStatus}</span>{/if}
          <FormError message={failure} />
        </div>
        <div class="actor-form-actions card-action-foot">
          <Button tone="quiet" onclick={closeAdd}>Cancel</Button>
          {#if !named}
            <Button
              type="submit"
              tone="signal"
              disabled={duplicateRole || (actorType === 'RepositoryRole' && !validRole)}
              >Add actor</Button
            >
          {/if}
        </div>
      </div>
    </form>
  {:else if actors.length === 0}
    <p class="empty-actors">No actors selected</p>
  {/if}
{/if}

<style>
  .actor-list {
    container: bypass-actors / inline-size;
  }
  .actor-symbol {
    align-items: center;
    color: var(--text-secondary);
    display: flex;
    flex: 0 0 var(--space-8);
    inline-size: var(--space-8);
    justify-content: center;
  }
  .availability-warning {
    color: var(--warning);
  }
  .actor-row {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    justify-content: space-between;
  }
  .actor-identity,
  .actor-actions {
    align-items: center;
    display: flex;
    gap: var(--space-3);
    min-inline-size: 0;
  }
  .actor-identity {
    flex: 1 1 12rem;
  }
  .actor-actions {
    flex-wrap: wrap;
  }
  /* Reserve the identity and the longest mode together; selecting another mode
     must not move that row's controls onto a different line from its neighbors. */
  @container bypass-actors (max-width: 30rem) {
    .actor-row {
      display: grid;
      grid-template-columns: minmax(0, 1fr);
    }
  }
  .actor-header,
  .directory-warning {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    justify-content: flex-end;
    padding-block-start: var(--space-3);
  }
  .actor-header {
    padding-block-start: 0;
    padding-block-end: var(--space-3);
  }
  .directory-warning {
    justify-content: space-between;
    color: var(--text-muted);
    font-size: var(--font-size-compact);
  }
  .empty-actors {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    margin: 0;
    padding-block: var(--space-3);
  }
  .actor-list-continues {
    --object-list-end: 0px;
  }
  .actor-form {
    display: grid;
    gap: var(--space-4);
    padding-block-start: var(--space-4);
  }
  .actor-fields {
    align-items: end;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
  }
  .actor-field {
    display: grid;
    gap: var(--space-2);
    min-inline-size: 0;
  }
  .identity-field {
    flex: 1 1 12rem;
  }
  .actor-field input {
    inline-size: 100%;
  }
  .actor-form-foot {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    justify-content: space-between;
  }
  .actor-form-status {
    display: grid;
    gap: var(--row-copy-gap);
    flex: 1 1 12rem;
    min-inline-size: 0;
  }
  .actor-form-status:empty {
    display: none;
  }
  .actor-form-status :global(.form-error) {
    margin: 0;
  }
  .actor-form-actions {
    display: flex;
    gap: var(--space-2);
    margin-inline-start: auto;
  }
  .result-row {
    align-items: center;
    display: flex;
    gap: var(--space-3);
    text-align: start;
  }
  .result-action {
    color: var(--text-secondary);
    margin-inline-start: auto;
  }
</style>

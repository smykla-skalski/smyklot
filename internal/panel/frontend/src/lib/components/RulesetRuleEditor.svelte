<script lang="ts">
  import { onDestroy } from 'svelte';
  import { numericValue } from '../merge';
  import {
    cloneSettingsJson,
    sameSettingsJson,
    type SettingsJson,
  } from '../settings-draft-storage';
  import { bypassActorId, bypassActorKey } from '../bypass-policy';
  import type {
    BypassActorIdentity,
    BypassActorLookup,
    SyncRulesetBypassActor,
    SyncRulesetRules,
  } from '../types';
  import {
    ALERT_LEVELS,
    SECURITY_LEVELS,
    addRuleItem,
    newRuleParameters,
    ruleItems,
    withMergeMethod,
    withRuleField,
    type EditableRuleKey,
    type RuleParameters,
  } from '../ruleset-rule-editor';
  import Modal from './Modal.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import Button from './Button.svelte';
  import IconButton from './IconButton.svelte';
  import Icon from './Icon.svelte';
  import Switch from './Switch.svelte';
  import Select from './Select.svelte';
  import Popover from './Popover.svelte';
  import FormError from './FormError.svelte';
  import Avatar from './Avatar.svelte';

  const {
    scope,
    rulesetName,
    rules,
    disabled,
    lookup,
    onApply,
  }: {
    scope: string;
    rulesetName: string;
    rules: SyncRulesetRules | undefined;
    disabled: boolean;
    lookup?: BypassActorLookup;
    onApply: (key: EditableRuleKey, parameters: RuleParameters) => boolean | void;
  } = $props();

  const TITLES = {
    pull_request: 'Require a pull request',
    required_status_checks: 'Require status checks',
    update: 'Restrict updates',
    code_scanning: 'Require code scanning',
  };
  const FLAGS = [
    {
      field: 'dismiss_stale_reviews_on_push',
      label: 'Dismiss stale approvals',
      help: 'Require fresh approval when new commits change the pull request',
    },
    { field: 'require_code_owner_review', label: "Require a code owner's review" },
    {
      field: 'require_last_push_approval',
      label: 'Require last push approval',
      help: 'Someone other than the last person to push must approve',
    },
    { field: 'required_review_thread_resolution', label: 'Resolve review conversations' },
  ];
  const METHODS = [
    { value: 'merge', label: 'Merge commit' },
    { value: 'squash', label: 'Squash' },
    { value: 'rebase', label: 'Rebase' },
  ];
  let open = $state(false);
  let key = $state<EditableRuleKey>('pull_request');
  let openingScope = $state('');
  let creating = $state(false);
  let original = $state.raw<RuleParameters>({});
  let draft = $state.raw<RuleParameters>({});
  let trigger = $state<HTMLElement | null>(null);
  let approvals = $state('0');
  let confirmingDiscard = $state(false);
  let stagingRejected = $state(false);
  let editingControl = $state<HTMLElement | null>(null);
  let adding = $state(false);
  let addName = $state('');
  let addError = $state<string | null>(null);
  let identities = $state.raw<BypassActorIdentity[]>([]);
  let loadingNames = $state(false);
  let namesRequest = 0;
  onDestroy(() => {
    namesRequest++;
  });
  const field = $derived(
    key === 'required_status_checks' ? 'required_status_checks' : 'code_scanning_tools',
  );
  const nameField = $derived(key === 'required_status_checks' ? 'context' : 'tool');
  const items = $derived(ruleItems(draft, field));
  const methods = $derived(
    (draft.allowed_merge_methods as string[] | undefined) ?? ['merge', 'squash', 'rebase'],
  );
  const problem = $derived.by(() => {
    if (key === 'pull_request') {
      const value = Number(approvals);
      if (approvals.trim() === '' || !Number.isInteger(value) || value < 0 || value > 10)
        return 'Choose a whole number from 0 to 10';
      if (methods.length === 0) return 'Choose at least one merge method';
    }
    if (key === 'required_status_checks' && items.length === 0)
      return 'Add at least one required check';
    if (key === 'code_scanning' && items.length === 0) return 'Add at least one scanning tool';
    return null;
  });

  function ruleChanged(): boolean {
    const current = rules?.[key];
    return creating
      ? current !== undefined
      : current === undefined ||
          !sameSettingsJson(current as SettingsJson, original as SettingsJson);
  }
  const conflict = $derived(
    stagingRejected
      ? 'This edit could not be staged · close and reopen the editor to review the current settings'
      : open && ruleChanged()
        ? 'This rule changed elsewhere · close and reopen the editor to review its current values'
        : null,
  );
  const changed = $derived(
    !sameSettingsJson(draft as SettingsJson, original as SettingsJson) ||
      (key === 'pull_request' &&
        problem === 'Choose a whole number from 0 to 10' &&
        approvals !== String(numericValue(original.required_approving_review_count) ?? 0)) ||
      addName.trim() !== '',
  );

  function beforeClose(): boolean {
    if (!changed) return true;
    if (!confirmingDiscard) {
      const active = document.activeElement;
      if (active instanceof HTMLElement && active.closest('#ruleset-rule-editor'))
        editingControl = active;
      else if (!editingControl?.isConnected)
        editingControl = document.getElementById('ruleset-rule-editor');
      confirmingDiscard = true;
    }
    return false;
  }

  function requestClose(): void {
    if (beforeClose()) close();
  }

  function keepEditing(): void {
    confirmingDiscard = false;
  }

  export function show(nextKey: EditableRuleKey, element?: HTMLElement): void {
    if (disabled) return;
    confirmingDiscard = false;
    stagingRejected = false;
    editingControl = null;
    key = nextKey;
    openingScope = scope;
    creating = rules?.[key] === undefined;
    original = cloneSettingsJson(
      ((rules?.[key] as RuleParameters | undefined) ?? newRuleParameters(key)) as SettingsJson,
    ) as RuleParameters;
    draft = original;
    approvals = String(numericValue(original.required_approving_review_count) ?? 0);
    trigger = element ?? null;
    adding = false;
    addName = '';
    addError = null;
    open = true;
    identities = [];
    loadingNames = false;
    const request = ++namesRequest;
    if (key === 'required_status_checks' && lookup && items.some((item) => pinnedApp(item)))
      void loadNames(lookup, request, openingScope);
  }

  async function loadNames(load: BypassActorLookup, request: number, owner: string): Promise<void> {
    loadingNames = true;
    try {
      const response = await load();
      if (open && request === namesRequest && scope === owner && lookup === load)
        identities = response.items;
    } catch {
      // The saved pin remains authoritative when its display name cannot be read.
    } finally {
      if (request === namesRequest) loadingNames = false;
    }
  }

  function close(): void {
    const destination = trigger;
    const owner = openingScope;
    confirmingDiscard = false;
    adding = false;
    open = false;
    namesRequest++;
    loadingNames = false;
    queueMicrotask(() => {
      if (!open && scope === owner && destination?.isConnected)
        destination.focus({ preventScroll: true });
    });
  }
  function apply(): void {
    if (!open || disabled || openingScope !== scope || problem || ruleChanged() || stagingRejected)
      return;
    if (onApply(key, draft) === false) {
      stagingRejected = true;
      return;
    }
    close();
  }
  function boolean(field: string, value: boolean): void {
    if (!disabled) draft = withRuleField(original, draft, field, value, original[field] === true);
  }
  function setApprovals(value: string): void {
    approvals = value;
    const count = Number(value);
    if (!disabled && value.trim() !== '' && Number.isInteger(count) && count >= 0 && count <= 10) {
      draft = withRuleField(
        original,
        draft,
        'required_approving_review_count',
        count,
        numericValue(original.required_approving_review_count) ?? 0,
      );
    }
  }
  function removeItem(index: number): void {
    if (!disabled)
      draft = withRuleField(
        original,
        draft,
        field,
        items.filter((_, at) => at !== index),
      );
  }
  function addItem(event: SubmitEvent): void {
    event.preventDefault();
    if (disabled) return;
    const name = addName.trim();
    if (!name) {
      addError = 'Enter a name';
      return;
    }
    if (items.some((item) => item[nameField] === name)) {
      addError = 'That name is already required';
      return;
    }
    draft = addRuleItem(
      original,
      draft,
      field,
      nameField,
      name,
      key === 'code_scanning'
        ? { alerts_threshold: 'errors', security_alerts_threshold: 'high_or_higher' }
        : {},
    );
    adding = false;
    addName = '';
    addError = null;
  }
  function threshold(index: number, property: string, value: string): void {
    if (disabled) return;
    const current = items[index]!;
    const initial =
      ruleItems(original, field).find((held) => held.tool === current.tool) ?? current;
    const next = withRuleField(initial, current, property, value);
    draft = withRuleField(
      original,
      draft,
      field,
      items.map((item, at) => (at === index ? next : item)),
    );
  }
  function pinnedApp(
    item: RuleParameters,
  ): { id: string; identity: BypassActorIdentity | undefined } | null {
    const id = item.integration_id as SyncRulesetBypassActor['actor_id'] | undefined;
    if (id == null) return null;
    const actor = { actor_type: 'Integration', actor_id: id };
    const reference = bypassActorId(actor);
    if (reference === '0') return null;
    return {
      id: reference,
      identity: identities.find((held) => bypassActorKey(held) === bypassActorKey(actor)),
    };
  }
  $effect(() => {
    if (open && scope !== openingScope) close();
  });
</script>

<!--
@component
A bounded rule draft. Done stages complete parameters; Cancel never changes the
ruleset. Incidental dismissal asks before discarding private edits and keeps the
mounted editor behind the question. Unknown fields and pinned child records keep
their original numeric tokens.
-->
<Modal
  id="ruleset-rule-editor"
  {open}
  title={TITLES[key]}
  description={rulesetName}
  variant="inspector"
  returnFocus={trigger}
  onClose={close}
  {beforeClose}
>
  {#snippet headerExtra()}<div class="editor-header-actions">
      {#if disabled}<span class="card-meta">Read only</span>{/if}<IconButton
        toolbar
        icon="close"
        label="Close rule editor"
        onclick={requestClose}
      />
    </div>{/snippet}
  <div
    class="form-stack"
    onfocusin={(event) => {
      if (event.target instanceof HTMLElement) editingControl = event.target;
    }}
  >
    {#if key === 'pull_request'}
      <label class="form-field"
        ><span class="form-label">Approvals required</span><input
          class="text-input text-inline approval-count"
          type="number"
          min="0"
          max="10"
          step="1"
          value={approvals}
          {disabled}
          aria-label="Approvals required"
          oninput={(event) => setApprovals(event.currentTarget.value)}
        /><span class="form-help">Zero requires a pull request without requiring an approval</span
        ></label
      >
      {#each FLAGS as flag (flag.field)}
        <div class="form-row">
          <div class="form-field">
            <span class="form-label">{flag.label}</span>
            {#if flag.help}<p class="form-help">{flag.help}</p>{/if}
          </div>
          <Switch
            bare
            checked={draft[flag.field] === true}
            {disabled}
            label={flag.label}
            onToggle={(value) => boolean(flag.field, value)}
          />
        </div>
      {/each}
      <fieldset class="method-choices form-field">
        <legend class="form-label">Allowed merge methods</legend>
        <div class="method-options">
          {#each METHODS as method (method.value)}<label class="check-item"
              ><input
                type="checkbox"
                checked={methods.includes(method.value)}
                {disabled}
                onchange={() => {
                  if (!disabled) draft = withMergeMethod(original, draft, method.value);
                }}
              /><span class="check-box" aria-hidden="true"><Icon name="check" size="micro" /></span
              ><span>{method.label}</span></label
            >{/each}
        </div>
      </fieldset>
    {:else if key === 'required_status_checks' || key === 'code_scanning'}
      <div class="form-row">
        <span class="form-label"
          >{key === 'required_status_checks' ? 'Required checks' : 'Required tools'}</span
        ><Popover
          role="dialog"
          label={key === 'required_status_checks' ? 'Check to add' : 'Tool to add'}
          bind:open={adding}
          onopen={() => {
            addName = '';
            addError = null;
          }}
        >
          {#snippet trigger(attributes)}<Button {...attributes} class="" tone="quiet" {disabled}
              >{#snippet icon()}<Icon name="plus" size="sm" />{/snippet}{key ===
              'required_status_checks'
                ? 'Add a check'
                : 'Add a tool'}</Button
            >{/snippet}
          <form class="form-stack add-entry" onsubmit={addItem}>
            <label class="form-field"
              ><span class="form-label"
                >{key === 'required_status_checks' ? 'Check name' : 'Tool name'}</span
              ><input
                class="text-input"
                bind:value={addName}
                aria-label={key === 'required_status_checks' ? 'Check to add' : 'Tool to add'}
                spellcheck="false"
                {disabled}
              /></label
            ><FormError message={addError} /><Button type="submit" tone="quiet" {disabled}
              >Add</Button
            >
          </form>
        </Popover>
      </div>
      {#each items as item, index (index)}
        <section class="form-stack" aria-label={String(item[nameField])}>
          <div class="form-row">
            <div class="form-field">
              <h3 class="form-label item-name">{String(item[nameField])}</h3>
              {#if key === 'required_status_checks'}
                {@const app = pinnedApp(item)}
                {#if app}
                  <div class="pin-source">
                    {#if app.identity}
                      <Avatar
                        account={{
                          id: app.id,
                          provider: 'github',
                          subject_id: app.id,
                          login: app.identity.slug,
                          display_name: app.identity.name,
                          avatar_url: app.identity.avatar_url,
                        }}
                        size={20}
                        shape="workspace"
                      />
                    {/if}
                    <span class="form-field">
                      <span class="form-help"
                        >{loadingNames
                          ? 'Looking up app name'
                          : (app.identity?.name ?? 'App name unavailable')}</span
                      >
                      {#if !app.identity && !loadingNames}<span class="form-help pin-reference"
                          >App ID {app.id}</span
                        >{/if}
                    </span>
                  </div>
                {/if}
              {/if}
            </div>
            <IconButton
              toolbar
              icon="close"
              label={`Remove ${String(item[nameField])}`}
              {disabled}
              onclick={() => removeItem(index)}
            />
          </div>
          {#if key === 'code_scanning'}<div class="form-row">
              <span class="form-label">Alerts</span><Select
                aria-label={`${String(item.tool)} alerts`}
                value={String(item.alerts_threshold)}
                options={ALERT_LEVELS}
                {disabled}
                onValueChange={(value) => threshold(index, 'alerts_threshold', value)}
              />
            </div>
            <div class="form-row">
              <span class="form-label">Security alerts</span><Select
                aria-label={`${String(item.tool)} security alerts`}
                value={String(item.security_alerts_threshold)}
                options={SECURITY_LEVELS}
                {disabled}
                onValueChange={(value) => threshold(index, 'security_alerts_threshold', value)}
              />
            </div>{/if}
        </section>
      {/each}
      {#if key === 'required_status_checks'}<div class="form-row">
          <div class="form-field">
            <span class="form-label">Require branches to be up to date</span>
            <p class="form-help">Test changes against the latest target branch before merging</p>
          </div>
          <Switch
            bare
            checked={draft.strict_required_status_checks_policy === true}
            {disabled}
            label="Require branches to be up to date"
            onToggle={(value) => boolean('strict_required_status_checks_policy', value)}
          />
        </div>
        <div class="form-row">
          <span class="form-label">Skip checks on branch creation</span><Switch
            bare
            checked={draft.do_not_enforce_on_create === true}
            {disabled}
            label="Skip on branch creation"
            onToggle={(value) => boolean('do_not_enforce_on_create', value)}
          />
        </div>{:else}<p class="form-help">
          Merging waits for each tool and is blocked by alerts at or above its selected thresholds
        </p>{/if}
    {:else}<div class="form-row">
        <div class="form-field">
          <span class="form-label">Allow syncing from upstream</span>
          <p class="form-help">Allow the branch to pull changes from its upstream repository</p>
        </div>
        <Switch
          bare
          checked={draft.update_allows_fetch_and_merge === true}
          {disabled}
          label="Allow syncing from upstream"
          onToggle={(value) => boolean('update_allows_fetch_and_merge', value)}
        />
      </div>{/if}
    <FormError message={conflict ?? problem} />
  </div>
  <ConfirmDialog
    id="discard-rule-changes"
    open={confirmingDiscard}
    title="Discard rule changes?"
    returnFocus={editingControl}
    onClose={keepEditing}
    onConfirm={close}
    confirmLabel="Discard changes"
    confirmTone="stop"
    cancelLabel="Keep editing"
  >
    <p class="form-help">Your changes to this rule will be lost</p>
  </ConfirmDialog>
  {#snippet footer()}<Button tone="quiet" onclick={close}>Cancel</Button><Button
      tone="signal"
      disabled={disabled || problem !== null || conflict !== null}
      onclick={apply}>Done</Button
    >{/snippet}
</Modal>

<style>
  .editor-header-actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .approval-count {
    inline-size: 5ch;
  }
  .method-choices {
    border: 0;
    margin: 0;
    min-inline-size: 0;
    padding: 0;
  }
  .method-choices legend {
    padding: 0;
  }
  .method-options {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3) var(--space-4);
  }
  .add-entry {
    inline-size: min(22rem, calc(100vw - var(--space-8)));
  }
  .item-name {
    color: var(--text-primary);
    margin: 0;
    overflow-wrap: anywhere;
  }
  .pin-source {
    align-items: center;
    display: flex;
    gap: var(--space-2);
    min-inline-size: 0;
  }
  .pin-reference {
    overflow-wrap: anywhere;
  }
</style>

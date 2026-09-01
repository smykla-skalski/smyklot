<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';

  import { CONFIG_KEYS } from '../config';
  import { durationParts, type DurationUnit } from '../duration';
  import { formatBytes, formatElapsed, formatLatency } from '../format';
  import {
    FORMATTING_FIELDS,
    formattingOverrideCount,
    type FormattingFieldKey,
    type FormattingPatch,
  } from '../formatting';
  import type { RootRuntimeSection } from '../routes';
  import {
    adoptRuntimeSettings,
    applyRuntimeConfigPatch,
    overlayRuntimeSettings,
    parseRuntimeSettingsDraftDocument,
    ROOT_SETTINGS_SCOPE,
    RUNTIME_DURATION_SPECS,
    runtimeConfigPatch,
    runtimeDurationEditor,
    runtimeDurationSeconds,
    runtimeSettingsDraftDocument,
    stageRuntimeSettingsControl,
    type RuntimeDurationKey,
    type RuntimeDurationSpec,
    type RuntimeSettingsControlId,
  } from '../runtime-settings';
  import { getSettingsDraftRegistry } from '../settings-drafts.svelte';
  import type {
    ConfigKey,
    ConfigPatch,
    RootRuntimeSettings,
    RootRuntimeSettingsInput,
  } from '../types';
  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import ClippedLabel from './ClippedLabel.svelte';
  import ConfigEditor from './ConfigEditor.svelte';
  import FormattingEditor from './FormattingEditor.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import FormError from './FormError.svelte';
  import Icon from './Icon.svelte';
  import Popover from './Popover.svelte';
  import RootPageHeader from './RootPageHeader.svelte';
  import StatusPill from './StatusPill.svelte';

  const LOG_LEVELS = [
    { value: 'debug', label: 'Debug' },
    { value: 'info', label: 'Info' },
    { value: 'warn', label: 'Warn' },
    { value: 'error', label: 'Error' },
  ] as const;
  const UNIT_WORDS: Record<DurationUnit, string> = {
    seconds: 'seconds',
    minutes: 'minutes',
    hours: 'hours',
    days: 'days',
  };
  const UNIT_SECONDS = { minutes: 60, hours: 3_600, days: 86_400 } as const;
  const SECTION_COPY: Record<
    RootRuntimeSection,
    {
      ariaLabel: string;
      title: string;
      subtitle: string;
      loading: string;
      unavailable: string;
    }
  > = {
    settings: {
      ariaLabel: 'Service settings',
      title: 'Service settings',
      subtitle: 'Service configuration and the defaults every workspace inherits',
      loading: 'Reading the settings…',
      unavailable: 'The settings could not be read',
    },
    service: {
      ariaLabel: 'Service health',
      title: 'Service health',
      subtitle: 'The running service, its listeners, credentials, and state store',
      loading: 'Reading the service…',
      unavailable: 'The service could not describe itself',
    },
  };

  const {
    section,
    fetchSettings,
    saveSettings,
  }: {
    section: RootRuntimeSection;
    fetchSettings: () => Promise<RootRuntimeSettings>;
    saveSettings?: (input: RootRuntimeSettingsInput) => Promise<RootRuntimeSettings>;
  } = $props();

  const drafts = getSettingsDraftRegistry();
  const queryClient = useQueryClient();
  const settingsQuery = createQuery(() => ({
    queryKey: ['root-settings'],
    queryFn: fetchSettings,
  }));
  const canonicalSettings = $derived<RootRuntimeSettings | null>(settingsQuery.data ?? null);
  const document = $derived(
    canonicalSettings === null ? null : runtimeSettingsDraftDocument(drafts, canonicalSettings),
  );
  const settings = $derived(
    canonicalSettings === null || document === null
      ? null
      : overlayRuntimeSettings(canonicalSettings, document),
  );
  const loading = $derived(settingsQuery.isPending);
  const saving = $derived(drafts.operation(ROOT_SETTINGS_SCOPE).saving);
  const settingsDirty = $derived(drafts.hasDirty(ROOT_SETTINGS_SCOPE));
  let actionFailure = $state<string | null>(null);
  let pauseDialogOpen = $state(false);
  let pauseSaving = $state(false);
  const failure = $derived(
    actionFailure ??
      (settingsQuery.error === null
        ? null
        : settingsQuery.error instanceof Error
          ? settingsQuery.error.message
          : String(settingsQuery.error)),
  );

  const runtimeOverridden = $derived(
    settings === null
      ? 0
      : [
          settings.log_level.override !== null,
          settings.session_lifetime.override_seconds !== null,
        ].filter(Boolean).length,
  );

  const dirtyConfigKeys = $derived(
    CONFIG_KEYS.filter((key) => controlDirty(`runtime.bot_config.${key}`)),
  );
  const dirtyFormattingKeys = $derived(
    FORMATTING_FIELDS.filter((field) => controlDirty(`runtime.bot_config.${field.key}`)).map(
      (field) => field.key,
    ),
  );

  $effect(() => {
    const current = canonicalSettings;
    if (current === null) return;
    untrack(() => adoptRuntimeSettings(drafts, current));
  });

  async function load(): Promise<void> {
    actionFailure = null;
    await settingsQuery.refetch();
  }

  async function setBackgroundWorkPaused(paused: boolean): Promise<void> {
    const current = canonicalSettings;
    if (current === null || saveSettings === undefined || pauseSaving) return;
    pauseSaving = true;
    actionFailure = null;
    try {
      const updated = await saveSettings({
        background_work_paused: paused,
        bot_config: current.behavior_defaults.override,
        log_level: current.log_level.override,
        reaction_poll_interval_seconds: current.reaction_poll_interval.override_seconds,
        merge_after_ci_quiet_period_seconds: current.merge_after_ci_quiet_period.override_seconds,
        path_index_interval_seconds: current.path_index_interval.override_seconds,
        session_ttl_seconds: current.session_lifetime.override_seconds,
        expected_revision: current.revision,
      });
      queryClient.setQueryData(['root-settings'], updated);
      pauseDialogOpen = false;
    } catch (cause) {
      actionFailure = cause instanceof Error ? cause.message : String(cause);
      await settingsQuery.refetch();
    } finally {
      pauseSaving = false;
    }
  }

  const SESSION_SPEC = RUNTIME_DURATION_SPECS.session_ttl_seconds;

  function controlDirty(controlId: RuntimeSettingsControlId): boolean {
    return drafts.isControlDirty(ROOT_SETTINGS_SCOPE, controlId);
  }

  function stage(nextValue: unknown, controlId: RuntimeSettingsControlId): boolean {
    const current = canonicalSettings;
    const next = parseRuntimeSettingsDraftDocument(nextValue);
    if (
      current === null ||
      next === null ||
      !stageRuntimeSettingsControl(drafts, current, next, controlId)
    ) {
      actionFailure = 'This setting is not valid';
      return false;
    }
    actionFailure = null;
    return true;
  }

  function updateBehavior(patch: ConfigPatch, changedKey: ConfigKey): void {
    const current = canonicalSettings;
    if (current === null || document === null) return;
    stage(
      {
        ...document,
        bot_config: applyRuntimeConfigPatch(current.behavior_defaults.deployment, patch),
      },
      `runtime.bot_config.${changedKey}`,
    );
  }

  function updateFormatting(formatting: FormattingPatch, changedKey: FormattingFieldKey): void {
    const current = canonicalSettings;
    if (current === null || document === null) return;
    const patch = runtimeConfigPatch(
      current.behavior_defaults.deployment,
      current.behavior_defaults.override,
    );
    if (formattingOverrideCount(formatting) === 0) delete patch.formatting;
    else patch.formatting = formatting;
    stage(
      {
        ...document,
        bot_config: applyRuntimeConfigPatch(current.behavior_defaults.deployment, patch),
      },
      `runtime.bot_config.${changedKey}`,
    );
  }

  function setFormattingValidity(valid: boolean): void {
    drafts.setValidationProblem(
      ROOT_SETTINGS_SCOPE,
      'runtime.bot_config.formatting',
      valid ? null : 'Formatting widths must be whole numbers within their documented bounds',
    );
  }

  function setLogLevel(value: string | null): void {
    if (document === null) return;
    stage({ ...document, log_level: value }, 'runtime.log_level');
  }

  function durationFallback(key: RuntimeDurationKey): number {
    if (canonicalSettings === null) return 0;
    switch (key) {
      case 'reaction_poll_interval_seconds':
        return canonicalSettings.reaction_poll_interval.deployment_seconds;
      case 'merge_after_ci_quiet_period_seconds':
        return canonicalSettings.merge_after_ci_quiet_period.deployment_seconds;
      case 'path_index_interval_seconds':
        return canonicalSettings.path_index_interval.deployment_seconds;
      case 'session_ttl_seconds':
        return canonicalSettings.session_lifetime.deployment_seconds;
    }
  }

  function durationMaximum(spec: RuntimeDurationSpec): number {
    return spec.key === 'path_index_interval_seconds'
      ? (canonicalSettings?.path_index_interval.max_seconds ?? spec.maximumSeconds)
      : spec.maximumSeconds;
  }

  function durationProblem(spec: RuntimeDurationSpec): string | null {
    if (document === null || document[spec.key].editor === null) return null;
    return runtimeDurationSeconds(document[spec.key], spec, durationMaximum(spec)) === undefined
      ? spec.problem
      : null;
  }

  function typeAmount(spec: RuntimeDurationSpec, value: string): void {
    if (document === null) return;
    const held = document[spec.key];
    const editor = runtimeDurationEditor(held, spec, durationFallback(spec.key));
    stage(
      { ...document, [spec.key]: { ...held, editor: { amount: value, unit: editor.unit } } },
      `runtime.${spec.key}`,
    );
  }

  function pickUnit(spec: RuntimeDurationSpec, unit: DurationUnit): void {
    if (document === null) return;
    const held = document[spec.key];
    const editor = runtimeDurationEditor(held, spec, durationFallback(spec.key));
    stage(
      { ...document, [spec.key]: { ...held, editor: { amount: editor.amount, unit } } },
      `runtime.${spec.key}`,
    );
  }

  function setDuration(spec: RuntimeDurationSpec, seconds: number | null): void {
    if (document === null) return;
    stage(
      { ...document, [spec.key]: { override_seconds: seconds, editor: null } },
      `runtime.${spec.key}`,
    );
  }

  function formatDuration(seconds: number, units: readonly DurationUnit[]): string {
    if (seconds === 0) return 'disabled';
    const { amount, unit } = durationParts(seconds, units);
    const word = UNIT_WORDS[unit];
    return `${amount} ${amount === 1 ? word.slice(0, -1) : word}`;
  }

  function capitalize(value: string): string {
    return value.charAt(0).toUpperCase() + value.slice(1);
  }

  /* What each credential is FOR, because "OAuth" on its own tells an operator
     nothing about what breaks when it is missing. */
  const CREDENTIALS = [
    {
      key: 'webhook',
      name: 'Webhook secret',
      why: 'Verifies every delivery GitHub sends',
    },
    {
      key: 'app',
      name: 'GitHub App key',
      why: 'Signs the tokens every workspace call runs on',
    },
    {
      key: 'oauth',
      name: 'OAuth app secret',
      why: 'Signs sign-in sessions. If it is wrong, sign-in fails',
    },
  ] as const;

  function waitsSentence(connections: { wait_count: number; wait_ms: number }): string {
    if (connections.wait_count === 0) {
      return 'No caller has waited for a free connection since this service started';
    }

    return `Callers have waited ${connections.wait_count} ${connections.wait_count === 1 ? 'time' : 'times'} since startup, for ${formatElapsed(connections.wait_ms)} in total`;
  }

  function formatUptime(seconds: number): string {
    const days = Math.floor(seconds / UNIT_SECONDS.days);
    const hours = Math.floor((seconds % UNIT_SECONDS.days) / UNIT_SECONDS.hours);
    if (days > 0) return `${days}d ${hours}h`;
    const minutes = Math.floor(seconds / UNIT_SECONDS.minutes);
    return hours > 0 ? `${hours}h ${minutes % 60}m` : `${minutes}m`;
  }
</script>

<!--
@component
The service's own runtime settings, which are the only ones in the panel that are not
about a repository. Sections rather than one long form, because these are unrelated
knobs that happen to live in the same process.

`saveSettings` is optional and its absence is the read-only view: an operator who may
see how the service is configured without being able to change it gets the same page
without the composer.
-->

{#snippet durationValue(spec: RuntimeDurationSpec, label: string)}
  {@const value = document?.[spec.key]}
  {@const editor =
    value === undefined
      ? { amount: '', unit: spec.units[0] }
      : runtimeDurationEditor(value, spec, durationFallback(spec.key))}
  <input
    class="num-inline"
    inputmode="numeric"
    maxlength="32"
    aria-label="{label} amount"
    aria-invalid={durationProblem(spec) !== null}
    value={editor.amount}
    disabled={saving}
    oninput={(event) => typeAmount(spec, event.currentTarget.value)}
  />
  <Popover role="listbox" label="{label} unit" align="end" itemSelector=".menu-item">
    {#snippet trigger(attributes)}
      <button
        {...attributes}
        class="value-select"
        type="button"
        aria-label="{label} unit"
        disabled={saving}
      >
        <span class="t">{UNIT_WORDS[editor.unit]}</span>
      </button>
    {/snippet}
    <div class="menu-list">
      {#each spec.units as unit (unit)}
        <button
          class="menu-item"
          role="option"
          aria-selected={editor.unit === unit}
          onclick={() => pickUnit(spec, unit)}
        >
          <span class="menu-check">
            {#if editor.unit === unit}<Icon name="check" size="base" />{/if}
          </span>
          <ClippedLabel class="mi-label" text={UNIT_WORDS[unit]} />
        </button>
      {/each}
    </div>
  </Popover>
{/snippet}

<section class="root-settings" aria-label={SECTION_COPY[section].ariaLabel}>
  <RootPageHeader title={SECTION_COPY[section].title} subtitle={SECTION_COPY[section].subtitle}>
    {#if section === 'settings'}
      <StatusPill dot={settingsDirty}>Changes wait for Save</StatusPill>
    {/if}
  </RootPageHeader>
  {#if loading && settings === null}
    <div class="settings-state" role="status">{SECTION_COPY[section].loading}</div>
  {:else if settings === null}
    <div class="settings-state settings-error" role="alert">
      <strong>{SECTION_COPY[section].unavailable}</strong>
      <span>{failure}</span>
      <Button onclick={() => void load()}>Try again</Button>
    </div>
  {:else}
    {@const current = settings}
    {#if failure !== null}
      <FormError message={failure} />
    {/if}

    {#if section === 'settings'}
      {#if saveSettings !== undefined}
        <Card
          class={current.background_work_paused ? 'emergency-card paused' : 'emergency-card'}
          labelledby="background-work-control"
        >
          <div class="emergency-copy">
            <div class="emergency-heading">
              <!-- h2, like every card title on this page: these groups are the page's
                   own sections, so an h3 under the page's h1 announced a level that was
                   not there and then went back up to h2 for the cards below. -->
              <h2 class="group-name" id="background-work-control">Automatic background work</h2>
              <StatusPill dot state={current.background_work_paused ? 'warning' : 'healthy'}>
                {current.background_work_paused ? 'Paused' : 'Running'}
              </StatusPill>
            </div>
            <p class="group-note emergency-note">
              {#if current.background_work_paused}
                Queue items remain durable, but webhook delivery, pending CI, sync, and maintenance
                will not start new work
              {:else}
                Every job starts the work that is due. Use this control to stop automatic dispatch
                without taking the panel or webhook intake offline
              {/if}
            </p>
          </div>
          {#if current.background_work_paused}
            <Button
              tone="signal"
              disabled={pauseSaving}
              onclick={() => void setBackgroundWorkPaused(false)}
            >
              {pauseSaving ? 'Resuming…' : 'Resume automatic work'}
            </Button>
          {:else}
            <Button
              tone="stop-quiet"
              disabled={pauseSaving}
              onclick={() => (pauseDialogOpen = true)}
            >
              Pause automatic work
            </Button>
          {/if}
        </Card>
      {/if}

      <!-- The service's own settings lead the page, and what workspaces inherit
           follows: a reader who came to change the log level was reading past ten
           cards of defaults to find two rows about the process they are running. -->
      <Card class="group-card" labelledby="root-runtime">
        <div class="group-head">
          <h2 class="group-name" id="root-runtime">Runtime</h2>
          <span class="group-tally">{runtimeOverridden} of 2 overridden</span>
        </div>
        <p class="group-note">
          Applied to the running process without a restart. Background-work cadence and timing are
          managed in <a href="/root/schedules">Schedules</a>
        </p>
        <div class="policy-rows">
          <div
            class={['policy-row', { 'is-unsaved': controlDirty('runtime.log_level') }]}
            data-unsaved={controlDirty('runtime.log_level') || undefined}
          >
            <span class="setting-say">
              <span class="setting-name">Log level</span>
              <span class="setting-why">Updates the process logger without restarting Smyklot</span>
            </span>
            {#if current.log_level.override === null}
              <span class="policy-value">
                <span class="setting-unmanaged"
                  >From the deployment: {capitalize(current.log_level.deployment)}</span
                >
              </span>
              <button
                class="setting-clear"
                title="Override the deployment log level"
                disabled={saving}
                onclick={() => setLogLevel(current.log_level.deployment)}
              >
                <Icon name="plus" size="micro" />
              </button>
            {:else}
              <span class="policy-value">
                <Popover
                  role="listbox"
                  label="Log level choices"
                  align="end"
                  itemSelector=".menu-item"
                >
                  {#snippet trigger(attributes)}
                    <button
                      {...attributes}
                      class="value-select"
                      type="button"
                      aria-label="Runtime log level"
                      disabled={saving}
                    >
                      <span class="t"
                        >{capitalize(
                          current.log_level.override ?? current.log_level.deployment,
                        )}</span
                      >
                    </button>
                  {/snippet}
                  <div class="menu-list">
                    {#each LOG_LEVELS as option (option.value)}
                      <button
                        class="menu-item"
                        role="option"
                        aria-selected={current.log_level.override === option.value}
                        onclick={() => setLogLevel(option.value)}
                      >
                        <span class="menu-check">
                          {#if current.log_level.override === option.value}<Icon
                              name="check"
                              size="base"
                            />{/if}
                        </span>
                        <ClippedLabel class="mi-label" text={option.label} />
                      </button>
                    {/each}
                  </div>
                </Popover>
              </span>
              <button
                class="setting-clear"
                title="Stop overriding - take the value from the deployment"
                disabled={saving}
                onclick={() => setLogLevel(null)}
              >
                <Icon name="close" size="micro" />
              </button>
            {/if}
          </div>

          <div
            class={[
              'policy-row',
              {
                'is-unsaved': controlDirty('runtime.session_ttl_seconds'),
                'is-invalid': durationProblem(SESSION_SPEC) !== null,
              },
            ]}
            data-unsaved={controlDirty('runtime.session_ttl_seconds') || undefined}
          >
            <span class="setting-say">
              <span class="setting-name">Sign-in sessions</span>
              <span class="setting-why"
                >Shorter limits end active sessions sooner; longer limits affect new sessions</span
              >
              {#if durationProblem(SESSION_SPEC) !== null}
                <span class="setting-problem">{durationProblem(SESSION_SPEC)}</span>
              {/if}
            </span>
            {#if current.session_lifetime.override_seconds === null}
              <span class="policy-value">
                <span class="setting-unmanaged"
                  >From the deployment: {formatDuration(
                    current.session_lifetime.deployment_seconds,
                    SESSION_SPEC.units,
                  )}</span
                >
              </span>
              <button
                class="setting-clear"
                title="Override the deployment session lifetime"
                disabled={saving}
                onclick={() =>
                  setDuration(SESSION_SPEC, current.session_lifetime.deployment_seconds)}
              >
                <Icon name="plus" size="micro" />
              </button>
            {:else}
              <span class="policy-value">
                {@render durationValue(SESSION_SPEC, 'Session lifetime')}
              </span>
              <button
                class="setting-clear"
                title="Stop overriding - take the value from the deployment"
                disabled={saving}
                onclick={() => setDuration(SESSION_SPEC, null)}
              >
                <Icon name="close" size="micro" />
              </button>
            {/if}
          </div>
        </div>
      </Card>

      <ConfigEditor
        patch={runtimeConfigPatch(
          current.behavior_defaults.deployment,
          current.behavior_defaults.override,
        )}
        inherited={current.behavior_defaults.deployment}
        scope="runtime"
        idPrefix="root"
        disabled={saving}
        dirtyKeys={dirtyConfigKeys}
        onChange={updateBehavior}
      />

      <FormattingEditor
        patch={runtimeConfigPatch(
          current.behavior_defaults.deployment,
          current.behavior_defaults.override,
        ).formatting ?? {}}
        inherited={current.behavior_defaults.deployment.formatting}
        scope="runtime"
        idPrefix="root"
        disabled={saving}
        dirtyKeys={dirtyFormattingKeys}
        onChange={updateFormatting}
        onValidity={setFormattingValidity}
      />

      {#if current.updated_at !== undefined}
        <p class="updated-note">
          Service settings last changed <time datetime={current.updated_at}
            >{new Date(current.updated_at).toLocaleString()}</time
          >
          {#if current.updated_by !== undefined}
            by @{current.updated_by.login}{/if}
        </p>
      {/if}
    {/if}

    {#if section === 'service'}
      {@const database = current.service.database}
      <!-- One page and one voice. The service was one card and its database another,
           each opening with a card title that repeated the page's own, and each
           laying its facts out as a grid of labels: "WAITS SINCE START 2 · 41 ms"
           is two numbers and a riddle. Every fact here is a row that says what it
           means. -->
      <Card>
        <div class="setting-rows">
          <div class="setting-row">
            <span class="setting-say"><span class="setting-name">Version</span></span>
            <span class="setting-value">
              <span class="setting-fact"
                >{current.service.version || 'development'} · up
                <span class="nowrap-atom">{formatUptime(current.service.uptime_seconds)}</span
                ></span
              >
            </span>
          </div>
          <div class="setting-row">
            <span class="setting-say">
              <span class="setting-name">Listeners</span>
              <span class="setting-why"
                >The webhook port faces GitHub; the admin port stays inside</span
              >
            </span>
            <span class="setting-value">
              <span class="setting-fact"
                ><span class="nowrap-atom">public {current.service.listeners.public}</span> ·
                <span class="nowrap-atom">admin {current.service.listeners.admin}</span></span
              >
            </span>
          </div>
          <div class="setting-row">
            <span class="setting-say">
              <span class="setting-name">Paths</span>
              <span class="setting-why">Where this panel and GitHub's deliveries arrive</span>
            </span>
            <span class="setting-value">
              <span class="setting-fact"
                ><span class="nowrap-atom">panel {current.service.public_paths.panel}</span> ·
                <span class="nowrap-atom">webhook {current.service.public_paths.webhook}</span
                ></span
              >
            </span>
          </div>
        </div>
      </Card>

      <!-- Every credential the panel depends on, with its state: sign-in's behaviour
           and this page can never tell different stories. -->
      <Card>
        <div class="card-head"><h2 class="card-title">Credentials</h2></div>
        <div class="setting-rows">
          {#each CREDENTIALS as credential (credential.key)}
            {@const present = current.service.credential_presence[credential.key]}
            <div class="setting-row">
              <span class="setting-say">
                <span class="setting-name">{credential.name}</span>
                <span class="setting-why">{credential.why}</span>
              </span>
              <span class="setting-value">
                <span class="mx-mark" class:mx-instep={present} class:mx-refused={!present}>
                  <span class="t">{present ? 'Configured' : 'Missing'}</span>
                </span>
              </span>
            </div>
          {/each}
        </div>
      </Card>

      <Card>
        <div class="card-head">
          <h2 class="card-title">Database</h2>
          <StatusPill dot state={database.state}>{database.state}</StatusPill>
        </div>
        <div class="setting-rows">
          <div class="setting-row">
            <span class="setting-say"><span class="setting-name">Engine</span></span>
            <span class="setting-value">
              <span class="setting-fact"
                >{database.engine}{database.version === '' ? '' : ` ${database.version}`} · schema
                {database.schema_version} ·
                <span class="nowrap-atom">{formatBytes(database.size_bytes)}</span></span
              >
            </span>
          </div>
          <div class="setting-row">
            <span class="setting-say"><span class="setting-name">Responsiveness</span></span>
            <span class="setting-value">
              <span class="setting-fact"
                >answers in
                <span class="nowrap-atom">{formatLatency(database.latency_ms)}</span></span
              >
            </span>
          </div>
          <div class="setting-row">
            <span class="setting-say">
              <span class="setting-name">Connections</span>
              <!-- The waits are cumulative, unlike the counts beside them: a pool
                   that reads idle now may still have held the service up an hour
                   ago, which is the only reason this number is worth keeping. -->
              <span class="setting-why">{waitsSentence(database.connections)}</span>
            </span>
            <span class="setting-value">
              <span class="setting-fact nowrap-atom"
                >{database.connections.in_use} of {database.connections.max} open</span
              >
            </span>
          </div>
          {#if database.detail !== undefined}
            <div class="setting-row">
              <span class="setting-say">
                <span class="setting-name">Reported</span>
                <span class="setting-why">{database.detail}</span>
              </span>
              <span class="setting-value"></span>
            </div>
          {/if}
        </div>
      </Card>

      <Card>
        <div class="card-head"><h2 class="card-title">Where it talks to GitHub</h2></div>
        <div class="setting-rows">
          {#each [{ name: 'API', value: current.service.provider_endpoints.api }, { name: 'Sign-in', value: current.service.provider_endpoints.authorize }, { name: 'Token exchange', value: current.service.provider_endpoints.token }] as endpoint (endpoint.name)}
            <div class="setting-row">
              <span class="setting-say"><span class="setting-name">{endpoint.name}</span></span>
              <span class="setting-value">
                <span class="setting-fact"><code>{endpoint.value}</code></span>
              </span>
            </div>
          {/each}
        </div>
      </Card>
    {/if}
  {/if}
</section>

<ConfirmDialog
  id="pause-background-work"
  open={pauseDialogOpen}
  title="Pause automatic background work?"
  description="This is an immediate service-wide safety control."
  onClose={() => (pauseDialogOpen = false)}
  onConfirm={() => void setBackgroundWorkPaused(true)}
  confirmLabel="Pause background work"
  busyLabel="Pausing…"
  confirmTone="stop"
  busy={pauseSaving}
>
  <p class="confirm-copy">
    No job will take on new work. Work already running may finish, and incoming webhooks remain
    stored for later delivery. An operator can resume dispatch from this page.
  </p>
</ConfirmDialog>

<style>
  .root-settings {
    display: grid;
    gap: var(--space-4);
    /* This stack spaces its children, so the head it holds owes only the rest of
       its exit - see `--head-exit-gap` in `app.css`. */
    --head-exit-gap: var(--space-4);
  }

  .group-head {
    align-items: end;
    display: flex;
    gap: var(--space-3);
    justify-content: space-between;
    margin-bottom: var(--space-2);
  }

  /* `.emergency-card` and its paused tint are in `app.css`. They are worn by `Card`'s own
     root element, which never carries this component's scope class - so written here they
     matched nothing, the row never became a row, and the button sat under the sentence
     with no space between them. */

  .emergency-copy {
    display: grid;
    gap: var(--space-3);
    min-width: 0;
  }

  .emergency-heading {
    align-items: center;
    display: flex;
    gap: var(--space-3);
    /* The line this heading's companions are held to - its own title's cap. */
    --head-line: 12px;
  }

  /* No measure, like every other card note: the copy column is already bounded by the
     act beside it, and a cap on top of that broke one sentence across two lines. */
  .emergency-note {
    margin: 0;
  }

  .confirm-copy {
    color: var(--text-muted);
    margin: 0;
  }

  .group-name {
    font-size: var(--font-size-title);
    font-weight: 600;
    margin: 0;
    /* This heading's own cap, which is not the card title's: the type is a step smaller.
       Forced to the card's 13px line the box gained 1.1px it could not fill, and since a
       block box fills from the top the words sat half a pixel above their own centre -
       which the alignment sweep reads, correctly, as a row that does not centre. */
    min-block-size: 12px;
    text-box: trim-both cap alphabetic;
  }

  @media (max-width: 720px) {
    .emergency-card {
      align-items: stretch;
      flex-direction: column;
    }

    .emergency-card :global(.btn) {
      justify-content: center;
      width: 100%;
    }
  }

  .group-tally {
    color: var(--text-muted);
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    min-block-size: 8px;
    text-box: trim-both cap alphabetic;
  }

  .policy-row.is-invalid {
    background: color-mix(in srgb, var(--danger) 7%, var(--surface-base));
    box-shadow: inset 2px 0 var(--danger);
  }

  /* A third line in the say stack, on the same rhythm the sentence above it takes from
     the shared law - `1cap` read in this line's own voice, not the row's. */
  .setting-problem {
    color: var(--danger);
    font-size: var(--font-size-compact);
    margin-block-start: calc(var(--leading-compact) - 1cap);
  }

  .value-select {
    align-items: center;
    appearance: none;
    background:
      linear-gradient(45deg, transparent 49%, var(--text-secondary) 51%) calc(100% - 14px) 55% / 5px
        5px no-repeat,
      linear-gradient(135deg, var(--text-secondary) 49%, transparent 51%) calc(100% - 9px) 55% / 5px
        5px no-repeat,
      var(--control-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text-primary);
    cursor: pointer;
    display: inline-flex;
    font-size: var(--font-size-control);
    min-block-size: var(--tier-quiet);
    padding: 0 1.5rem 0 var(--space-2);
  }

  /* Ink-true, so the chosen word shares the row's centre with the say
     beside it rather than riding its line box's leading. */
  .value-select .t {
    text-box: trim-both cap alphabetic;
  }

  .value-select[data-state='open'] {
    background:
      linear-gradient(45deg, transparent 49%, var(--text-secondary) 51%) calc(100% - 14px) 55% / 5px
        5px no-repeat,
      linear-gradient(135deg, var(--text-secondary) 49%, transparent 51%) calc(100% - 9px) 55% / 5px
        5px no-repeat,
      var(--control-bg-pressed);
  }

  .menu-item {
    align-items: center;
    background: none;
    border: 0;
    border-radius: 6px;
    block-size: 32px;
    color: var(--text-primary);
    cursor: pointer;
    display: flex;
    font-size: var(--font-size-control);
    gap: var(--space-2);
    inline-size: 100%;
    padding-inline: var(--space-3);
    text-align: start;
  }

  .menu-item:hover {
    background: var(--interactive-hover-layer);
  }

  .menu-item:focus-visible {
    background: var(--interactive-hover-layer);
    outline: none;
  }

  .menu-item:active {
    background: var(--interactive-pressed);
  }

  .menu-check {
    display: inline-flex;
    flex: none;
    inline-size: 16px;
    justify-content: center;
  }

  .menu-item :global(.mi-label) {
    min-inline-size: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .num-inline {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text-primary);
    font-family: var(--mono);
    font-size: var(--font-size-control);
    min-block-size: var(--tier-quiet);
    padding: 0 var(--space-2);
    text-align: end;
    width: 5rem;
  }

  .num-inline:focus-visible {
    border-color: var(--brand-action);
    outline: 2px solid var(--focus);
  }

  .num-inline[aria-invalid='true'] {
    border-color: var(--danger);
  }

  .pill {
    align-items: center;
    block-size: var(--tier-mark);
    border-radius: var(--radius-chip);
    display: inline-flex;
    font-size: var(--font-size-micro);
    font-weight: 600;
    gap: 0.25rem;
    line-height: var(--leading-flat);
    padding: 0 0.5rem;
  }

  .pill .t {
    display: block;
    text-box: trim-both cap alphabetic;
  }

  .pill-success {
    background: var(--success-tint);
    color: var(--success);
  }

  .pill-muted {
    background: var(--surface-inset);
    color: var(--text-muted);
  }

  .updated-note {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    justify-self: end;
    margin: 0;
  }

  .settings-state {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-surface);
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
    gap: var(--space-3);
    justify-content: center;
    min-height: 12rem;
    padding: var(--space-5);
  }

  .settings-error {
    align-items: flex-start;
    display: flex;
    flex-direction: column;
  }

  /* On a phone the head's three parts cannot share one line - the tally or
     pill drops under the title instead of holding the card wide. */
  @media (max-width: 30rem) {
    .group-head {
      flex-wrap: wrap;
    }
  }
</style>

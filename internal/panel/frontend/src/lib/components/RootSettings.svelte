<script lang="ts">
  import { createMutation, createQuery, useQueryClient } from '@tanstack/svelte-query';

  import { formatBytes, formatElapsed, formatLatency } from '../format';
  import type {
    ConfigPatch,
    ConfigValues,
    RootRuntimeSettings,
    RootRuntimeSettingsInput,
  } from '../types';
  import Button from './Button.svelte';
  import ClippedLabel from './ClippedLabel.svelte';
  import ConfigEditor from './ConfigEditor.svelte';
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
  const UNIT_SECONDS = {
    seconds: 1,
    minutes: 60,
    hours: 3_600,
    days: 86_400,
  } as const;
  type DurationUnit = keyof typeof UNIT_SECONDS;
  const UNIT_WORDS: Record<DurationUnit, string> = {
    seconds: 'seconds',
    minutes: 'minutes',
    hours: 'hours',
    days: 'days',
  };

  const {
    rootRole,
    fetchSettings,
    updateSettings,
  }: {
    rootRole: string;
    fetchSettings: () => Promise<RootRuntimeSettings>;
    updateSettings: (input: RootRuntimeSettingsInput) => Promise<RootRuntimeSettings>;
  } = $props();

  const queryClient = useQueryClient();
  const settingsQuery = createQuery(() => ({
    queryKey: ['root-settings'],
    queryFn: fetchSettings,
  }));
  const settingsMutation = createMutation(() => ({
    mutationFn: updateSettings,
    onSuccess: (updated) => queryClient.setQueryData(['root-settings'], updated),
  }));
  const settings = $derived<RootRuntimeSettings | null>(settingsQuery.data ?? null);
  const loading = $derived(settingsQuery.isPending);
  const saving = $derived(settingsMutation.isPending);
  let actionFailure = $state<string | null>(null);
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
          settings.reaction_poll_interval.override_seconds !== null,
          settings.merge_after_ci_quiet_period.override_seconds !== null,
          settings.path_index_interval.override_seconds !== null,
          settings.session_lifetime.override_seconds !== null,
        ].filter(Boolean).length,
  );

  async function load(): Promise<void> {
    actionFailure = null;
    await settingsQuery.refetch();
  }

  /* ---------- The saved receipts, one per card ---------- */

  let runtimeSavedOn = $state(false);
  let runtimeTimer: ReturnType<typeof setTimeout> | undefined;

  function whisper(): void {
    runtimeSavedOn = true;
    clearTimeout(runtimeTimer);
    runtimeTimer = setTimeout(() => (runtimeSavedOn = false), 1400);
  }

  async function update(
    change: Partial<
      Pick<
        RootRuntimeSettingsInput,
        | 'bot_config'
        | 'log_level'
        | 'reaction_poll_interval_seconds'
        | 'session_ttl_seconds'
        | 'merge_after_ci_quiet_period_seconds'
        | 'path_index_interval_seconds'
      >
    >,
  ): Promise<void> {
    if (settings === null || saving) return;
    actionFailure = null;
    try {
      await settingsMutation.mutateAsync({
        bot_config: settings.behavior_defaults.override,
        log_level: settings.log_level.override,
        reaction_poll_interval_seconds: settings.reaction_poll_interval.override_seconds,
        merge_after_ci_quiet_period_seconds: settings.merge_after_ci_quiet_period.override_seconds,
        path_index_interval_seconds: settings.path_index_interval.override_seconds,
        session_ttl_seconds: settings.session_lifetime.override_seconds,
        expected_revision: settings.revision,
        ...change,
      });
      whisper();
    } catch (error) {
      actionFailure = errorMessage(error);
      throw error;
    }
  }

  /* The behavior card reports through its own receipt; a refusal still lands
     on this page's failure line. */
  async function saveBehavior(patch: ConfigPatch): Promise<void> {
    if (settings === null) return;
    const botConfig =
      Object.keys(patch).length === 0
        ? null
        : applyConfigPatch(settings.behavior_defaults.deployment, patch);
    actionFailure = null;
    try {
      await settingsMutation.mutateAsync({
        bot_config: botConfig,
        log_level: settings.log_level.override,
        reaction_poll_interval_seconds: settings.reaction_poll_interval.override_seconds,
        merge_after_ci_quiet_period_seconds: settings.merge_after_ci_quiet_period.override_seconds,
        path_index_interval_seconds: settings.path_index_interval.override_seconds,
        session_ttl_seconds: settings.session_lifetime.override_seconds,
        expected_revision: settings.revision,
      });
    } catch (error) {
      actionFailure = errorMessage(error);
      throw error;
    }
  }

  function quietly(work: Promise<void>): void {
    /* update() already parked the refusal on the failure line. */
    work.catch(() => {});
  }

  /* ---------- The three duration rows ---------- */

  interface DurationSpec {
    /** The input change carrying this row's seconds to the service. */
    key:
      | 'reaction_poll_interval_seconds'
      | 'merge_after_ci_quiet_period_seconds'
      | 'path_index_interval_seconds'
      | 'session_ttl_seconds';
    units: readonly DurationUnit[];
    min: number;
    max: number;
    refused: string;
  }

  const POLL_SPEC: DurationSpec = {
    key: 'reaction_poll_interval_seconds',
    units: ['seconds', 'minutes', 'hours'],
    min: 1,
    max: 24 * UNIT_SECONDS.hours,
    refused: 'Reaction sweep interval must be between 1 second and 24 hours',
  };
  const QUIET_SPEC: DurationSpec = {
    key: 'merge_after_ci_quiet_period_seconds',
    units: ['seconds', 'minutes', 'hours'],
    min: 1,
    max: 24 * UNIT_SECONDS.hours,
    refused: 'Merge-after-CI quiet period must be between 1 second and 24 hours',
  };
  const PATH_INDEX_SPEC: DurationSpec = {
    key: 'path_index_interval_seconds',
    units: ['minutes', 'hours', 'days'],
    min: 60,
    /* Replaced at save time by the bound the service sends - see boundOf. */
    max: 7 * UNIT_SECONDS.days,
    refused: 'Path index interval must be between 1 minute and the service ceiling',
  };
  const SESSION_SPEC: DurationSpec = {
    key: 'session_ttl_seconds',
    units: ['minutes', 'hours', 'days'],
    min: 60,
    max: 30 * UNIT_SECONDS.days,
    refused: 'Session lifetime must be between 1 minute and 30 days',
  };

  /* One draft per duration row, alive only while the typing rests. */
  const SAVE_REST_MS = 900;
  const drafts = $state<Record<string, string>>({});
  const draftUnits = $state<Record<string, DurationUnit>>({});
  const timers: Record<string, ReturnType<typeof setTimeout>> = {};

  function durationParts(
    seconds: number,
    units: readonly DurationUnit[],
  ): {
    amount: number;
    unit: DurationUnit;
  } {
    for (const unit of [...units].reverse()) {
      if (seconds % UNIT_SECONDS[unit] === 0 && seconds >= UNIT_SECONDS[unit]) {
        return { amount: seconds / UNIT_SECONDS[unit], unit };
      }
    }
    return { amount: seconds, unit: units[0] };
  }

  function shownAmount(spec: DurationSpec, seconds: number): string {
    return drafts[spec.key] ?? durationParts(seconds, spec.units).amount.toString();
  }

  function shownUnit(spec: DurationSpec, seconds: number): DurationUnit {
    return draftUnits[spec.key] ?? durationParts(seconds, spec.units).unit;
  }

  function typeAmount(spec: DurationSpec, seconds: number, value: string): void {
    drafts[spec.key] = value;
    draftUnits[spec.key] = shownUnit(spec, seconds);
    clearTimeout(timers[spec.key]);
    timers[spec.key] = setTimeout(() => saveDuration(spec), SAVE_REST_MS);
  }

  function pickUnit(spec: DurationSpec, seconds: number, unit: DurationUnit): void {
    drafts[spec.key] = shownAmount(spec, seconds);
    draftUnits[spec.key] = unit;
    saveDuration(spec);
  }

  /** The service's own ceiling where it sends one; the spec's otherwise. */
  function boundOf(spec: DurationSpec): number {
    if (spec.key === 'path_index_interval_seconds') {
      return settings?.path_index_interval.max_seconds ?? spec.max;
    }
    return spec.max;
  }

  function saveDuration(spec: DurationSpec): void {
    clearTimeout(timers[spec.key]);
    delete timers[spec.key];
    const raw = drafts[spec.key];
    const unit = draftUnits[spec.key];
    if (raw === undefined || unit === undefined) return;
    const seconds = Math.round(Number(raw) * UNIT_SECONDS[unit]);
    if (!Number.isFinite(seconds) || seconds < spec.min || seconds > boundOf(spec)) {
      actionFailure = spec.refused;
      return;
    }
    delete drafts[spec.key];
    delete draftUnits[spec.key];
    quietly(update({ [spec.key]: seconds }));
  }

  function clearDrafts(spec: DurationSpec): void {
    clearTimeout(timers[spec.key]);
    delete timers[spec.key];
    delete drafts[spec.key];
    delete draftUnits[spec.key];
  }

  function formatDuration(seconds: number, units: readonly DurationUnit[]): string {
    if (seconds === 0) return 'disabled';
    const { amount, unit } = durationParts(seconds, units);
    const word = UNIT_WORDS[unit];
    return `${amount} ${amount === 1 ? word.slice(0, -1) : word}`;
  }

  function applyConfigPatch(deployment: ConfigValues, patch: ConfigPatch): ConfigValues {
    return JSON.parse(JSON.stringify({ ...deployment, ...patch })) as ConfigValues;
  }

  function capitalize(value: string): string {
    return value.charAt(0).toUpperCase() + value.slice(1);
  }

  function formatUptime(seconds: number): string {
    const days = Math.floor(seconds / UNIT_SECONDS.days);
    const hours = Math.floor((seconds % UNIT_SECONDS.days) / UNIT_SECONDS.hours);
    if (days > 0) return `${days}d ${hours}h`;
    const minutes = Math.floor(seconds / UNIT_SECONDS.minutes);
    return hours > 0 ? `${hours}h ${minutes % 60}m` : `${minutes}m`;
  }

  function errorMessage(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
  }
</script>

{#snippet durationValue(spec: DurationSpec, overrideSeconds: number, label: string)}
  <input
    class="num-inline"
    inputmode="numeric"
    aria-label="{label} amount"
    value={shownAmount(spec, overrideSeconds)}
    disabled={saving && drafts[spec.key] === undefined}
    oninput={(event) => typeAmount(spec, overrideSeconds, event.currentTarget.value)}
    onblur={() => saveDuration(spec)}
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
        <span class="t">{UNIT_WORDS[shownUnit(spec, overrideSeconds)]}</span>
      </button>
    {/snippet}
    <div class="menu-list">
      {#each spec.units as unit (unit)}
        <button
          class="menu-item"
          role="option"
          aria-selected={shownUnit(spec, overrideSeconds) === unit}
          onclick={() => pickUnit(spec, overrideSeconds, unit)}
        >
          <span class="menu-check">
            {#if shownUnit(spec, overrideSeconds) === unit}<Icon name="check" size={16} />{/if}
          </span>
          <ClippedLabel class="mi-label" text={UNIT_WORDS[unit]} />
        </button>
      {/each}
    </div>
  </Popover>
{/snippet}

<section class="root-settings" aria-label="Root runtime settings">
  <RootPageHeader
    role={rootRole}
    title="Settings"
    subtitle="Runtime behavior and deployment-backed defaults"
  >
    <StatusPill dot live>Changes apply live</StatusPill>
  </RootPageHeader>
  {#if loading && settings === null}
    <div class="settings-state" role="status">Loading runtime settings…</div>
  {:else if settings === null}
    <div class="settings-state settings-error" role="alert">
      <strong>Runtime settings are unavailable</strong>
      <span>{failure}</span>
      <Button onclick={() => void load()}>Try again</Button>
    </div>
  {:else}
    {@const current = settings}
    {#if failure !== null}
      <FormError message={failure} />
    {/if}

    <ConfigEditor
      patch={current.behavior_defaults.override === null
        ? {}
        : (Object.fromEntries(
            Object.entries(current.behavior_defaults.override).filter(
              ([key, value]) =>
                JSON.stringify(value) !==
                JSON.stringify(current.behavior_defaults.deployment[key as keyof ConfigValues]),
            ),
          ) as ConfigPatch)}
      inherited={current.behavior_defaults.deployment}
      scope="runtime"
      idPrefix="root"
      disabled={saving}
      onSave={saveBehavior}
    />

    <section class="card group-card" aria-labelledby="root-runtime">
      <div class="group-head">
        <h3 class="group-name" id="root-runtime">Runtime</h3>
        <span class="save-whisper" class:is-on={runtimeSavedOn} role="status"
          ><Icon name="check" size={12} /><span class="t">Saved</span></span
        >
        <span class="group-tally">{runtimeOverridden} of 5 overridden</span>
      </div>
      <p class="group-note">Applied to the running process without a restart</p>
      <div class="policy-rows">
        <div class="policy-row">
          <span class="setting-say">
            <span class="setting-name">Log level</span>
            <span class="setting-why">Updates the process logger without restarting Smyklot</span>
          </span>
          {#if current.log_level.override === null}
            <span class="policy-value">
              <span class="setting-unmanaged"
                >Follows the deployment - {capitalize(current.log_level.deployment)}</span
              >
            </span>
            <button
              class="setting-clear"
              title="Override the deployment log level"
              disabled={saving}
              onclick={() => quietly(update({ log_level: current.log_level.deployment }))}
            >
              <Icon name="plus" size={10} />
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
                      onclick={() => quietly(update({ log_level: option.value }))}
                    >
                      <span class="menu-check">
                        {#if current.log_level.override === option.value}<Icon
                            name="check"
                            size={16}
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
              title="Stop overriding - follow the deployment configuration"
              disabled={saving}
              onclick={() => quietly(update({ log_level: null }))}
            >
              <Icon name="close" size={10} />
            </button>
          {/if}
        </div>

        <div class="policy-row">
          <span class="setting-say">
            <span class="setting-name">Reaction sweep</span>
            <span class="setting-why"
              >Checks reaction changes GitHub does not deliver through webhooks. Zero turns the
              sweep off</span
            >
          </span>
          {#if current.reaction_poll_interval.override_seconds === null}
            <span class="policy-value">
              <span class="setting-unmanaged"
                >Follows the deployment - every {formatDuration(
                  current.reaction_poll_interval.deployment_seconds,
                  POLL_SPEC.units,
                )}</span
              >
            </span>
            <button
              class="setting-clear"
              title="Override the deployment sweep interval"
              disabled={saving}
              onclick={() =>
                quietly(
                  update({
                    reaction_poll_interval_seconds:
                      current.reaction_poll_interval.deployment_seconds,
                  }),
                )}
            >
              <Icon name="plus" size={10} />
            </button>
          {:else if current.reaction_poll_interval.override_seconds === 0}
            <span class="policy-value">
              <span class="value-word">off</span>
              <Button
                tone="quiet"
                disabled={saving}
                onclick={() =>
                  quietly(
                    update({
                      reaction_poll_interval_seconds: Math.max(
                        current.reaction_poll_interval.deployment_seconds,
                        UNIT_SECONDS.minutes,
                      ),
                    }),
                  )}
              >
                Turn on
              </Button>
            </span>
            <button
              class="setting-clear"
              title="Stop overriding - follow the deployment configuration"
              disabled={saving}
              onclick={() => quietly(update({ reaction_poll_interval_seconds: null }))}
            >
              <Icon name="close" size={10} />
            </button>
          {:else}
            <span class="policy-value">
              {@render durationValue(
                POLL_SPEC,
                current.reaction_poll_interval.override_seconds,
                'Reaction sweep interval',
              )}
              <Button
                tone="quiet"
                disabled={saving}
                onclick={() => {
                  clearDrafts(POLL_SPEC);
                  quietly(update({ reaction_poll_interval_seconds: 0 }));
                }}
              >
                Turn off
              </Button>
            </span>
            <button
              class="setting-clear"
              title="Stop overriding - follow the deployment configuration"
              disabled={saving}
              onclick={() => {
                clearDrafts(POLL_SPEC);
                quietly(update({ reaction_poll_interval_seconds: null }));
              }}
            >
              <Icon name="close" size={10} />
            </button>
          {/if}
        </div>

        <div class="policy-row">
          <span class="setting-say">
            <span class="setting-name">Merge after CI</span>
            <span class="setting-why"
              >Requires checks to remain unchanged and passing before Smyklot merges</span
            >
          </span>
          {#if current.merge_after_ci_quiet_period.override_seconds === null}
            <span class="policy-value">
              <span class="setting-unmanaged"
                >Follows the deployment - {formatDuration(
                  current.merge_after_ci_quiet_period.deployment_seconds,
                  QUIET_SPEC.units,
                )}</span
              >
            </span>
            <button
              class="setting-clear"
              title="Override the deployment quiet period"
              disabled={saving}
              onclick={() =>
                quietly(
                  update({
                    merge_after_ci_quiet_period_seconds:
                      current.merge_after_ci_quiet_period.deployment_seconds,
                  }),
                )}
            >
              <Icon name="plus" size={10} />
            </button>
          {:else}
            <span class="policy-value">
              {@render durationValue(
                QUIET_SPEC,
                current.merge_after_ci_quiet_period.override_seconds,
                'Merge-after-CI quiet period',
              )}
            </span>
            <button
              class="setting-clear"
              title="Stop overriding - follow the deployment configuration"
              disabled={saving}
              onclick={() => {
                clearDrafts(QUIET_SPEC);
                quietly(update({ merge_after_ci_quiet_period_seconds: null }));
              }}
            >
              <Icon name="close" size={10} />
            </button>
          {/if}
        </div>

        <div class="policy-row">
          <span class="setting-say">
            <span class="setting-name">Path index</span>
            <span class="setting-why"
              >How often each repository's file list is read again for the finder and the plans</span
            >
          </span>
          {#if current.path_index_interval.override_seconds === null}
            <span class="policy-value">
              <span class="setting-unmanaged"
                >Follows the deployment - every {formatDuration(
                  current.path_index_interval.deployment_seconds,
                  PATH_INDEX_SPEC.units,
                )}</span
              >
            </span>
            <button
              class="setting-clear"
              title="Override the deployment index interval"
              disabled={saving}
              onclick={() =>
                quietly(
                  update({
                    path_index_interval_seconds: current.path_index_interval.deployment_seconds,
                  }),
                )}
            >
              <Icon name="plus" size={10} />
            </button>
          {:else}
            <span class="policy-value">
              {@render durationValue(
                PATH_INDEX_SPEC,
                current.path_index_interval.override_seconds,
                'Path index interval',
              )}
            </span>
            <button
              class="setting-clear"
              title="Stop overriding - follow the deployment configuration"
              disabled={saving}
              onclick={() => {
                clearDrafts(PATH_INDEX_SPEC);
                quietly(update({ path_index_interval_seconds: null }));
              }}
            >
              <Icon name="close" size={10} />
            </button>
          {/if}
        </div>

        <div class="policy-row">
          <span class="setting-say">
            <span class="setting-name">Panel sessions</span>
            <span class="setting-why"
              >Reductions shorten active sessions; increases apply to new sessions</span
            >
          </span>
          {#if current.session_lifetime.override_seconds === null}
            <span class="policy-value">
              <span class="setting-unmanaged"
                >Follows the deployment - {formatDuration(
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
                quietly(
                  update({
                    session_ttl_seconds: current.session_lifetime.deployment_seconds,
                  }),
                )}
            >
              <Icon name="plus" size={10} />
            </button>
          {:else}
            <span class="policy-value">
              {@render durationValue(
                SESSION_SPEC,
                current.session_lifetime.override_seconds,
                'Session lifetime',
              )}
            </span>
            <button
              class="setting-clear"
              title="Stop overriding - follow the deployment configuration"
              disabled={saving}
              onclick={() => {
                clearDrafts(SESSION_SPEC);
                quietly(update({ session_ttl_seconds: null }));
              }}
            >
              <Icon name="close" size={10} />
            </button>
          {/if}
        </div>
      </div>
    </section>

    <!-- The database has its own card, and with it the state pill that used to
         stand on the runtime one. A word describing storage over a list of
         listeners and endpoints named neither of them. -->
    <section class="card group-card" aria-labelledby="root-database">
      <div class="group-head">
        <h3 class="group-name" id="root-database">Database</h3>
        <StatusPill dot state={current.service.database.state}>
          {current.service.database.state}
        </StatusPill>
      </div>
      <dl class="service-grid">
        <div>
          <dt>Engine</dt>
          <dd>{current.service.database.engine}</dd>
        </div>
        <div>
          <dt>Server version</dt>
          <dd>{current.service.database.version || 'unknown'}</dd>
        </div>
        <div>
          <dt>Schema version</dt>
          <dd>{current.service.database.schema_version}</dd>
        </div>
        <div>
          <dt>Size</dt>
          <dd>{formatBytes(current.service.database.size_bytes)}</dd>
        </div>
        <div>
          <dt>Response</dt>
          <dd>{formatLatency(current.service.database.latency_ms)}</dd>
        </div>
        <div class="wide">
          <dt>Connections</dt>
          <dd>
            {current.service.database.connections.in_use} in use · {current.service.database
              .connections.open} open · {current.service.database.connections.max} maximum
          </dd>
        </div>
        <div>
          <!-- Cumulative, unlike the counts beside it: a pool that reads idle
               now may still have held the service up earlier. -->
          <dt>Waits since start</dt>
          <dd>
            {current.service.database.connections.wait_count} · {formatElapsed(
              current.service.database.connections.wait_ms,
            )}
          </dd>
        </div>
        {#if current.service.database.detail !== undefined}
          <div class="full">
            <dt>Reported</dt>
            <dd class="database-detail">{current.service.database.detail}</dd>
          </div>
        {/if}
      </dl>
    </section>

    <section class="card group-card" aria-labelledby="root-service">
      <div class="group-head">
        <h3 class="group-name" id="root-service">Service and deployment</h3>
      </div>
      <dl class="service-grid">
        <div>
          <dt>Version</dt>
          <dd>{current.service.version}</dd>
        </div>
        <div>
          <dt>Uptime</dt>
          <dd>{formatUptime(current.service.uptime_seconds)}</dd>
        </div>
        <div>
          <dt>Public listener</dt>
          <dd><code>{current.service.listeners.public}</code></dd>
        </div>
        <div>
          <dt>Admin listener</dt>
          <dd><code>{current.service.listeners.admin}</code></dd>
        </div>
        <div>
          <dt>Panel path</dt>
          <dd><code>{current.service.public_paths.panel}</code></dd>
        </div>
        <div>
          <dt>Webhook path</dt>
          <dd><code>{current.service.public_paths.webhook}</code></dd>
        </div>
        <div class="full">
          <dt>GitHub API</dt>
          <dd><code>{current.service.provider_endpoints.api}</code></dd>
        </div>
        <div class="full">
          <dt>OAuth authorization</dt>
          <dd><code>{current.service.provider_endpoints.authorize}</code></dd>
        </div>
        <div class="full">
          <dt>OAuth token exchange</dt>
          <dd><code>{current.service.provider_endpoints.token}</code></dd>
        </div>
        <div class="wide">
          <dt>Credentials present</dt>
          <dd class="credential-list">
            <span
              class="pill"
              class:pill-success={current.service.credential_presence.webhook}
              class:pill-muted={!current.service.credential_presence.webhook}
              ><span class="t">Webhook</span></span
            >
            <span
              class="pill"
              class:pill-success={current.service.credential_presence.app}
              class:pill-muted={!current.service.credential_presence.app}
              ><span class="t">GitHub App</span></span
            >
            <span
              class="pill"
              class:pill-success={current.service.credential_presence.oauth}
              class:pill-muted={!current.service.credential_presence.oauth}
              ><span class="t">OAuth</span></span
            >
          </dd>
        </div>
      </dl>
    </section>

    {#if current.updated_at !== undefined}
      <p class="updated-note">
        Last changed <time datetime={current.updated_at}
          >{new Date(current.updated_at).toLocaleString()}</time
        >
        {#if current.updated_by !== undefined}
          by @{current.updated_by.login}{/if}
      </p>
    {/if}
  {/if}
</section>

<style>
  .root-settings {
    display: grid;
    gap: var(--space-4);
  }

  .card {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    padding: var(--space-5);
  }

  .group-head {
    align-items: end;
    display: flex;
    gap: var(--space-3);
    justify-content: space-between;
    margin-bottom: var(--space-2);
  }

  .group-name {
    font-size: var(--font-size-title);
    font-weight: 600;
    margin: 0;
    min-block-size: 12px;
    text-box: trim-both cap alphabetic;
  }

  .group-tally {
    color: var(--text-muted);
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    min-block-size: 8px;
    text-box: trim-both cap alphabetic;
  }

  .group-note {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    margin: 0 0 var(--space-2);
    max-width: 60ch;
  }

  .save-whisper {
    align-items: center;
    background: var(--success-tint);
    block-size: 20px;
    border-radius: var(--radius-chip);
    color: var(--success);
    display: inline-flex;
    font-size: var(--font-size-micro);
    font-weight: 600;
    gap: 4px;
    margin-inline-start: auto;
    opacity: 0;
    padding: 0 0.5rem;
    transition: opacity var(--duration-fast) var(--ease-standard);
  }

  .save-whisper.is-on {
    opacity: 1;
  }

  .save-whisper .t {
    text-box: trim-both cap alphabetic;
  }

  .policy-rows {
    display: grid;
  }

  .policy-row {
    align-items: center;
    display: grid;
    gap: var(--space-2) var(--space-4);
    grid-template-columns: 1fr auto auto;
    margin-inline: calc(var(--space-2) * -1);
    min-block-size: 48px;
    /* The air around a drawn hairline is the card's own padding, on both
       sides; the edge rows shed it where no line follows, since the card
       edge already carries that inset. */
    padding: var(--space-5) var(--space-2);
    position: relative;
  }

  .policy-row:first-child {
    padding-block-start: var(--space-2);
  }

  .policy-row:last-child {
    padding-block-end: var(--space-2);
  }

  /* Every row owns the drawn hairline under itself; the last one stands
     down, so the card ends on its own padding. */
  .policy-row:not(:last-child)::after {
    background: var(--border-subtle);
    block-size: 1px;
    bottom: 0;
    content: '';
    inset-inline: var(--space-2);
    position: absolute;
  }

  .setting-say {
    display: grid;
    gap: var(--space-3);
  }

  .setting-name {
    font-size: var(--font-size-meta);
    font-weight: 600;
    min-block-size: 10px;
    text-box: trim-both cap alphabetic;
  }

  .setting-why {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    min-block-size: 9px;
    text-box: trim-both cap alphabetic;
  }

  .policy-value {
    align-items: center;
    display: flex;
    gap: var(--space-3);
    justify-self: end;
  }

  .value-word {
    color: var(--text-muted);
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    min-inline-size: 1.9rem;
    text-align: end;
    text-box: trim-both cap alphabetic;
  }

  .setting-unmanaged {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    font-style: normal;
    /* Ink-true, so the padding around the hairlines measures to the glyphs
       rather than to the line box's leading. */
    text-box: trim-both cap alphabetic;
  }

  .setting-clear {
    align-items: center;
    background: transparent;
    block-size: 26px;
    border: 0;
    border-radius: 50%;
    color: var(--text-muted);
    cursor: pointer;
    display: inline-flex;
    inline-size: 26px;
    justify-content: center;
    padding: 0;
  }

  .setting-clear:hover {
    background: var(--interactive-hover-layer);
    color: var(--text-primary);
  }

  .setting-clear:active {
    background: var(--interactive-pressed);
  }

  .policy-row .setting-clear {
    opacity: 0.45;
    transition: opacity var(--duration-fast) var(--ease-standard);
  }

  .policy-row:hover .setting-clear,
  .policy-row:focus-within .setting-clear {
    opacity: 1;
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
    min-block-size: 28px;
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
    min-block-size: 28px;
    padding: 0 var(--space-2);
    text-align: end;
    width: 5rem;
  }

  .num-inline:focus-visible {
    border-color: var(--brand-action);
    outline: 2px solid var(--brand);
  }

  .pill {
    align-items: center;
    block-size: 20px;
    border-radius: var(--radius-chip);
    display: inline-flex;
    font-size: var(--font-size-micro);
    font-weight: 600;
    gap: 0.25rem;
    line-height: 1;
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

  /* A definition list, not a wall of boxed tiles: every other read-only key/value
     block in the product (the overview's service card, the ownership legend, the
     audit record) is a plain dl with an uppercase micro key over a mono value.
     Boxing each field drew ten competing surfaces inside one plate and left
     holes wherever a row did not fill its four columns. */
  .service-grid {
    display: grid;
    gap: var(--space-4) var(--space-6);
    grid-template-columns: repeat(4, minmax(0, 1fr));
    margin: 0;
  }

  .service-grid > div {
    min-width: 0;
  }

  .service-grid .wide {
    grid-column: span 2;
  }

  .service-grid .full {
    grid-column: 1 / -1;
  }

  /* A driver's own words, which wrap and do not shorten. Everything else in
     this grid is a value that fits its cell. */
  .database-detail {
    overflow-wrap: anywhere;
    white-space: normal;
  }

  dt {
    color: var(--text-muted);
    font: 700 var(--font-size-micro) / 1.3 var(--sans);
    letter-spacing: 0.05em;
    text-transform: uppercase;
  }

  dd {
    font: 600 var(--font-size-compact) / 1.5 var(--mono);
    margin: 0.15rem 0 0;
    min-width: 0;
    overflow-wrap: anywhere;
  }

  /* The value is already mono; a nested code element would only re-declare it. */
  dd code {
    font: inherit;
  }

  .credential-list {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
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

  @media (max-width: 64rem) {
    .service-grid {
      grid-template-columns: 1fr 1fr;
    }
  }

  @media (max-width: 40rem) {
    .service-grid {
      grid-template-columns: 1fr;
    }

    .service-grid .wide {
      grid-column: auto;
    }
  }

  /* On a phone the head's three parts cannot share one line - the tally or
     pill drops under the title instead of holding the card wide. */
  @media (max-width: 30rem) {
    .group-head {
      flex-wrap: wrap;
    }

    /* The say keeps the line and the control moves under it - beside it,
       the copy was down to a word a line while the control still ran off
       the screen and took the layout viewport with it. */
    .policy-row {
      grid-template-columns: minmax(0, 1fr) auto;
    }

    .policy-row .setting-say {
      grid-column: 1;
      grid-row: 1;
    }

    .policy-row .setting-clear {
      grid-column: 2;
      grid-row: 1;
      opacity: 1;
    }

    .policy-row .policy-value {
      flex-wrap: wrap;
      grid-column: 1 / -1;
      justify-self: start;
    }
  }
</style>

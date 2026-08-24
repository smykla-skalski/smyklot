<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';

  import { CONFIG_KEYS } from '../config';
  import { durationParts, type DurationUnit } from '../duration';
  import { formatBytes, formatElapsed, formatLatency } from '../format';
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
  import type { ConfigKey, ConfigPatch, RootRuntimeSettings } from '../types';
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
      ariaLabel: 'Root runtime settings',
      title: 'Runtime settings',
      subtitle: 'Runtime behavior and deployment-backed defaults',
      loading: 'Loading runtime settings…',
      unavailable: 'Runtime settings are unavailable',
    },
    service: {
      ariaLabel: 'Root service and deployment',
      title: 'Service and deployment',
      subtitle: 'Running service identity, listeners, endpoints, and credentials',
      loading: 'Loading service information…',
      unavailable: 'Service information is unavailable',
    },
    database: {
      ariaLabel: 'Root database',
      title: 'Database',
      subtitle: 'Persistence health, capacity, and schema compatibility',
      loading: 'Loading database status…',
      unavailable: 'Database status is unavailable',
    },
  };

  const {
    section,
    rootRole,
    fetchSettings,
  }: {
    section: RootRuntimeSection;
    rootRole: string;
    fetchSettings: () => Promise<RootRuntimeSettings>;
  } = $props();

  const drafts = getSettingsDraftRegistry();
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

  $effect(() => {
    const current = canonicalSettings;
    if (current === null) return;
    untrack(() => adoptRuntimeSettings(drafts, current));
  });

  async function load(): Promise<void> {
    actionFailure = null;
    await settingsQuery.refetch();
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

  function formatUptime(seconds: number): string {
    const days = Math.floor(seconds / UNIT_SECONDS.days);
    const hours = Math.floor((seconds % UNIT_SECONDS.days) / UNIT_SECONDS.hours);
    if (days > 0) return `${days}d ${hours}h`;
    const minutes = Math.floor(seconds / UNIT_SECONDS.minutes);
    return hours > 0 ? `${hours}h ${minutes % 60}m` : `${minutes}m`;
  }
</script>

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
            {#if editor.unit === unit}<Icon name="check" size={16} />{/if}
          </span>
          <ClippedLabel class="mi-label" text={UNIT_WORDS[unit]} />
        </button>
      {/each}
    </div>
  </Popover>
{/snippet}

<section class="root-settings" aria-label={SECTION_COPY[section].ariaLabel}>
  <RootPageHeader
    role={rootRole}
    title={SECTION_COPY[section].title}
    subtitle={SECTION_COPY[section].subtitle}
  >
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

      <section class="card group-card" aria-labelledby="root-runtime">
        <div class="group-head">
          <h3 class="group-name" id="root-runtime">Runtime</h3>
          <span class="group-tally">{runtimeOverridden} of 2 overridden</span>
        </div>
        <p class="group-note">
          Applied to the running process without a restart. Background-work cadence and timing are
          managed in <a href="/root/schedules">Schedules</a>.
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
                  >Follows the deployment - {capitalize(current.log_level.deployment)}</span
                >
              </span>
              <button
                class="setting-clear"
                title="Override the deployment log level"
                disabled={saving}
                onclick={() => setLogLevel(current.log_level.deployment)}
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
                        onclick={() => setLogLevel(option.value)}
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
                onclick={() => setLogLevel(null)}
              >
                <Icon name="close" size={10} />
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
              <span class="setting-name">Panel sessions</span>
              <span class="setting-why"
                >Reductions shorten active sessions; increases apply to new sessions</span
              >
              {#if durationProblem(SESSION_SPEC) !== null}
                <span class="setting-problem">{durationProblem(SESSION_SPEC)}</span>
              {/if}
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
                  setDuration(SESSION_SPEC, current.session_lifetime.deployment_seconds)}
              >
                <Icon name="plus" size={10} />
              </button>
            {:else}
              <span class="policy-value">
                {@render durationValue(SESSION_SPEC, 'Session lifetime')}
              </span>
              <button
                class="setting-clear"
                title="Stop overriding - follow the deployment configuration"
                disabled={saving}
                onclick={() => setDuration(SESSION_SPEC, null)}
              >
                <Icon name="close" size={10} />
              </button>
            {/if}
          </div>
        </div>
      </section>

      {#if current.updated_at !== undefined}
        <p class="updated-note">
          Runtime settings last changed <time datetime={current.updated_at}
            >{new Date(current.updated_at).toLocaleString()}</time
          >
          {#if current.updated_by !== undefined}
            by @{current.updated_by.login}{/if}
        </p>
      {/if}
    {/if}

    {#if section === 'database'}
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
    {/if}

    {#if section === 'service'}
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

  .policy-row.is-unsaved {
    background: color-mix(in srgb, var(--brand-action-tint) 45%, transparent);
    box-shadow: inset 2px 0 var(--brand-action);
  }

  .policy-row.is-invalid {
    background: color-mix(in srgb, var(--danger) 7%, var(--surface-base));
    box-shadow: inset 2px 0 var(--danger);
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

  .setting-problem {
    color: var(--danger);
    font-size: var(--font-size-compact);
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

  .num-inline[aria-invalid='true'] {
    border-color: var(--danger);
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

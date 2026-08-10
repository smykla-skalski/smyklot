<script lang="ts">
  import type {
    ConfigPatch,
    ConfigValues,
    RootRuntimeSettings,
    RootRuntimeSettingsInput,
  } from '../lib/types';
  import ConfigEditor from './ConfigEditor.svelte';
  import Plate from './Plate.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  const LOG_LEVELS = [
    { value: 'default', label: 'Deployment' },
    { value: 'debug', label: 'Debug' },
    { value: 'info', label: 'Info' },
    { value: 'warn', label: 'Warn' },
    { value: 'error', label: 'Error' },
  ] as const;
  const SESSION_SOURCES = [
    { value: 'default', label: 'Deployment' },
    { value: 'custom', label: 'Custom' },
  ] as const;
  const POLL_SOURCES = [
    { value: 'default', label: 'Deployment' },
    { value: 'disabled', label: 'Disabled' },
    { value: 'custom', label: 'Custom' },
  ] as const;
  const POLL_UNITS = {
    seconds: 1,
    minutes: 60,
    hours: 3_600,
  } as const;
  const SESSION_UNITS = {
    minutes: 60,
    hours: 3_600,
    days: 86_400,
  } as const;
  type SessionUnit = keyof typeof SESSION_UNITS;
  type PollUnit = keyof typeof POLL_UNITS;

  const {
    refreshVersion,
    fetchSettings,
    updateSettings,
  }: {
    refreshVersion: number;
    fetchSettings: () => Promise<RootRuntimeSettings>;
    updateSettings: (input: RootRuntimeSettingsInput) => Promise<RootRuntimeSettings>;
  } = $props();

  let settings = $state<RootRuntimeSettings | null>(null);
  let loading = $state(true);
  let saving = $state(false);
  let failure = $state<string | null>(null);
  let sessionSource = $state<'default' | 'custom'>('default');
  let sessionAmount = $state(24);
  let sessionUnit = $state<SessionUnit>('hours');
  let pollSource = $state<'default' | 'disabled' | 'custom'>('default');
  let pollAmount = $state(5);
  let pollUnit = $state<PollUnit>('minutes');
  let receivedRevision = $state(-1);

  const behaviorPatch = $derived(
    settings === null
      ? {}
      : configDifference(
          settings.behavior_defaults.override,
          settings.behavior_defaults.deployment,
        ),
  );

  $effect(() => {
    void load(refreshVersion);
  });

  $effect(() => {
    if (settings === null || settings.revision === receivedRevision) return;
    receivedRevision = settings.revision;
    const seconds =
      settings.session_lifetime.override_seconds ?? settings.session_lifetime.effective_seconds;
    const display = durationParts(seconds);
    sessionSource = settings.session_lifetime.override_seconds === null ? 'default' : 'custom';
    sessionAmount = display.amount;
    sessionUnit = display.unit;
    const pollSeconds = settings.reaction_poll_interval.override_seconds;
    if (pollSeconds === null) {
      pollSource = 'default';
    } else if (pollSeconds === 0) {
      pollSource = 'disabled';
    } else {
      pollSource = 'custom';
      const pollDisplay = pollDurationParts(pollSeconds);
      pollAmount = pollDisplay.amount;
      pollUnit = pollDisplay.unit;
    }
  });

  async function load(version: number): Promise<void> {
    loading = settings === null;
    failure = null;
    try {
      const current = await fetchSettings();
      if (version !== refreshVersion) return;
      settings = current;
    } catch (error) {
      if (version === refreshVersion) failure = errorMessage(error);
    } finally {
      if (version === refreshVersion) loading = false;
    }
  }

  async function saveBehavior(patch: ConfigPatch): Promise<void> {
    if (settings === null) return;
    const botConfig =
      Object.keys(patch).length === 0
        ? null
        : applyConfigPatch(settings.behavior_defaults.deployment, patch);
    await update({ bot_config: botConfig });
  }

  async function selectLogLevel(value: string): Promise<void> {
    if (settings === null || saving) return;
    await update({ log_level: value === 'default' ? null : value });
  }

  async function selectSessionSource(value: string): Promise<void> {
    if (settings === null || saving || value === sessionSource) return;
    sessionSource = value === 'custom' ? 'custom' : 'default';
    if (sessionSource === 'default') await update({ session_ttl_seconds: null });
  }

  async function selectPollSource(value: string): Promise<void> {
    if (settings === null || saving || value === pollSource) return;
    const next = value === 'custom' ? 'custom' : value === 'disabled' ? 'disabled' : 'default';
    if (next === 'custom') {
      pollSource = next;
      return;
    }
    await update({ reaction_poll_interval_seconds: next === 'disabled' ? 0 : null });
  }

  async function savePollInterval(): Promise<void> {
    if (settings === null || saving || pollSource !== 'custom') return;
    const seconds = Math.round(pollAmount * POLL_UNITS[pollUnit]);
    if (!Number.isFinite(seconds) || seconds < 1 || seconds > 24 * SESSION_UNITS.hours) {
      failure = 'Reaction sweep interval must be between 1 second and 24 hours';
      return;
    }
    await update({ reaction_poll_interval_seconds: seconds });
  }

  async function saveSessionLifetime(): Promise<void> {
    if (settings === null || saving || sessionSource !== 'custom') return;
    const seconds = Math.round(sessionAmount * SESSION_UNITS[sessionUnit]);
    if (!Number.isFinite(seconds) || seconds < 60 || seconds > 30 * SESSION_UNITS.days) {
      failure = 'Session lifetime must be between 1 minute and 30 days';
      return;
    }
    await update({ session_ttl_seconds: seconds });
  }

  async function update(
    change: Partial<
      Pick<
        RootRuntimeSettingsInput,
        'bot_config' | 'log_level' | 'reaction_poll_interval_seconds' | 'session_ttl_seconds'
      >
    >,
  ): Promise<void> {
    if (settings === null || saving) return;
    saving = true;
    failure = null;
    try {
      settings = await updateSettings({
        bot_config: settings.behavior_defaults.override,
        log_level: settings.log_level.override,
        reaction_poll_interval_seconds: settings.reaction_poll_interval.override_seconds,
        session_ttl_seconds: settings.session_lifetime.override_seconds,
        expected_revision: settings.revision,
        ...change,
      });
    } catch (error) {
      failure = errorMessage(error);
    } finally {
      saving = false;
    }
  }

  function configDifference(value: ConfigValues | null, deployment: ConfigValues): ConfigPatch {
    if (value === null) return {};
    const difference: ConfigPatch = {};
    for (const key of Object.keys(deployment) as Array<keyof ConfigValues>) {
      if (JSON.stringify(value[key]) !== JSON.stringify(deployment[key])) {
        Object.assign(difference, { [key]: structuredClone(value[key]) });
      }
    }
    return difference;
  }

  function applyConfigPatch(deployment: ConfigValues, patch: ConfigPatch): ConfigValues {
    return structuredClone({ ...deployment, ...patch });
  }

  function durationParts(seconds: number): { amount: number; unit: SessionUnit } {
    if (seconds % SESSION_UNITS.days === 0) {
      return { amount: seconds / SESSION_UNITS.days, unit: 'days' };
    }
    if (seconds % SESSION_UNITS.hours === 0) {
      return { amount: seconds / SESSION_UNITS.hours, unit: 'hours' };
    }
    return { amount: seconds / SESSION_UNITS.minutes, unit: 'minutes' };
  }

  function formatDuration(seconds: number): string {
    const { amount, unit } = durationParts(seconds);
    const label = amount === 1 ? unit.slice(0, -1) : unit;
    return `${amount} ${label}`;
  }

  function pollDurationParts(seconds: number): { amount: number; unit: PollUnit } {
    if (seconds % POLL_UNITS.hours === 0) {
      return { amount: seconds / POLL_UNITS.hours, unit: 'hours' };
    }
    if (seconds % POLL_UNITS.minutes === 0) {
      return { amount: seconds / POLL_UNITS.minutes, unit: 'minutes' };
    }
    return { amount: seconds, unit: 'seconds' };
  }

  function formatPollInterval(seconds: number): string {
    if (seconds === 0) return 'Disabled';
    const { amount, unit } = pollDurationParts(seconds);
    const label = amount === 1 ? unit.slice(0, -1) : unit;
    return `${amount} ${label}`;
  }

  function formatUptime(seconds: number): string {
    const days = Math.floor(seconds / SESSION_UNITS.days);
    const hours = Math.floor((seconds % SESSION_UNITS.days) / SESSION_UNITS.hours);
    if (days > 0) return `${days}d ${hours}h`;
    const minutes = Math.floor(seconds / SESSION_UNITS.minutes);
    return hours > 0 ? `${hours}h ${minutes % 60}m` : `${minutes}m`;
  }

  function errorMessage(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
  }
</script>

<section class="root-settings" aria-label="Root runtime settings">
  {#if loading}
    <div class="settings-state" role="status">Loading runtime settings…</div>
  {:else if settings === null}
    <div class="settings-state settings-error" role="alert">
      <strong>Runtime settings are unavailable</strong>
      <span>{failure}</span>
      <button class="btn" type="button" onclick={() => void load(refreshVersion)}>Try again</button>
    </div>
  {:else}
    {@const current = settings}
    <div class="live-note">
      <span class="live-mark" aria-hidden="true"></span>
      <div>
        <strong>Changes apply live</strong>
        <p>Every connected panel receives updates through the authenticated event stream.</p>
      </div>
    </div>

    {#if failure !== null}
      <p class="form-error" role="alert">{failure}</p>
    {/if}

    <Plate label="Behavior defaults">
      {#snippet status()}
        <span class="source-label">
          {current.behavior_defaults.override === null ? 'Deployment' : 'Runtime override'}
        </span>
      {/snippet}
      <p class="section-intro">
        Application-wide defaults inherited by installations and repositories. Clearing every custom
        value returns this group to its deployment configuration.
      </p>
      <ConfigEditor
        patch={behaviorPatch}
        inherited={current.behavior_defaults.deployment}
        scope="runtime"
        idPrefix="root"
        disabled={saving}
        onSave={saveBehavior}
      />
    </Plate>

    <div class="runtime-grid">
      <Plate label="Reaction sweep">
        {#snippet status()}
          <span class="effective-value">
            Effective: {formatPollInterval(current.reaction_poll_interval.effective_seconds)}
          </span>
        {/snippet}
        <p class="section-intro">
          Checks reaction changes that GitHub does not deliver through webhooks. The interval
          changes without restarting Smyklot.
        </p>
        <div class="session-editor">
          <SegmentedControl
            name="root-poll-source"
            label="Reaction sweep interval source"
            options={POLL_SOURCES}
            value={pollSource}
            onSelect={(selection) => void selectPollSource(selection)}
            disabled={saving}
          />
          {#if pollSource === 'custom'}
            <form
              class="duration-form"
              aria-label="Custom reaction sweep interval"
              onsubmit={(event) => {
                event.preventDefault();
                void savePollInterval();
              }}
            >
              <label>
                <span class="visually-hidden">Reaction sweep interval</span>
                <input
                  class="text-input duration-input"
                  type="number"
                  min="1"
                  step="1"
                  bind:value={pollAmount}
                  disabled={saving}
                />
              </label>
              <label>
                <span class="visually-hidden">Reaction sweep interval unit</span>
                <select class="select-input" bind:value={pollUnit} disabled={saving}>
                  <option value="seconds">Seconds</option>
                  <option value="minutes">Minutes</option>
                  <option value="hours">Hours</option>
                </select>
              </label>
              <button class="btn btn-signal" type="submit" disabled={saving}>Apply</button>
            </form>
          {/if}
        </div>
      </Plate>

      <Plate label="Log level">
        {#snippet status()}
          <span class="effective-value">Effective: {current.log_level.effective}</span>
        {/snippet}
        <p class="section-intro">Updates the process logger without restarting Smyklot.</p>
        <SegmentedControl
          name="root-log-level"
          label="Runtime log level"
          options={LOG_LEVELS}
          value={current.log_level.override ?? 'default'}
          onSelect={(selection) => void selectLogLevel(selection)}
          disabled={saving}
        />
      </Plate>

      <Plate label="Panel sessions">
        {#snippet status()}
          <span class="effective-value">
            Effective: {formatDuration(current.session_lifetime.effective_seconds)}
          </span>
        {/snippet}
        <p class="section-intro">
          Reductions shorten active sessions. Increases apply only to newly created sessions.
        </p>
        <div class="session-editor">
          <SegmentedControl
            name="root-session-source"
            label="Session lifetime source"
            options={SESSION_SOURCES}
            value={sessionSource}
            onSelect={(selection) => void selectSessionSource(selection)}
            disabled={saving}
          />
          {#if sessionSource === 'custom'}
            <form
              class="duration-form"
              aria-label="Custom session lifetime"
              onsubmit={(event) => {
                event.preventDefault();
                void saveSessionLifetime();
              }}
            >
              <label>
                <span class="visually-hidden">Session lifetime</span>
                <input
                  class="text-input duration-input"
                  type="number"
                  min="1"
                  step="1"
                  bind:value={sessionAmount}
                  disabled={saving}
                />
              </label>
              <label>
                <span class="visually-hidden">Session lifetime unit</span>
                <select class="select-input" bind:value={sessionUnit} disabled={saving}>
                  <option value="minutes">Minutes</option>
                  <option value="hours">Hours</option>
                  <option value="days">Days</option>
                </select>
              </label>
              <button class="btn btn-signal" type="submit" disabled={saving}>Apply</button>
            </form>
          {/if}
        </div>
      </Plate>
    </div>

    <Plate label="Service and deployment">
      {#snippet status()}
        <span class="service-health"><span aria-hidden="true"></span>{current.service.storage}</span
        >
      {/snippet}
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
        <div>
          <dt>GitHub API</dt>
          <dd><code>{current.service.provider_endpoints.api}</code></dd>
        </div>
        <div>
          <dt>OAuth authorization</dt>
          <dd><code>{current.service.provider_endpoints.authorize}</code></dd>
        </div>
        <div class="wide">
          <dt>OAuth token exchange</dt>
          <dd><code>{current.service.provider_endpoints.token}</code></dd>
        </div>
        <div class="wide">
          <dt>Credentials present</dt>
          <dd class="credential-list">
            <span class:present={current.service.credential_presence.webhook}>Webhook</span>
            <span class:present={current.service.credential_presence.app}>GitHub App</span>
            <span class:present={current.service.credential_presence.oauth}>OAuth</span>
          </dd>
        </div>
      </dl>
    </Plate>

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

  .root-settings :global(.plate) {
    background: var(--surface-base);
    border-color: color-mix(in srgb, #8b5cf6 13%, var(--border-subtle));
  }

  .live-note,
  .settings-state,
  .duration-form,
  .credential-list,
  .service-health {
    align-items: center;
    display: flex;
  }

  .live-note {
    background: color-mix(in srgb, #8b5cf6 6%, var(--surface-base));
    border: 1px solid color-mix(in srgb, #8b5cf6 20%, var(--border-subtle));
    border-radius: var(--radius-surface);
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
  }

  .live-note p,
  .section-intro,
  .updated-note {
    color: var(--text-secondary);
    font-size: var(--font-size-small);
    margin: 0;
  }

  .live-mark,
  .service-health span {
    background: var(--signal);
    border-radius: 50%;
    box-shadow: 0 0 0 4px color-mix(in srgb, var(--signal) 13%, transparent);
    flex: none;
    height: 0.5rem;
    width: 0.5rem;
  }

  .form-error {
    color: var(--stop);
    font-size: var(--font-size-small);
    margin: 0;
  }

  .source-label,
  .effective-value,
  .service-health {
    color: var(--text-secondary);
    font-size: var(--font-size-small);
  }

  .section-intro {
    margin-bottom: var(--space-3);
  }

  .runtime-grid {
    display: grid;
    gap: var(--space-4);
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .session-editor {
    align-items: flex-start;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
  }

  .duration-form {
    gap: var(--space-2);
  }

  .duration-input {
    width: 6rem;
  }

  .service-health {
    gap: var(--space-2);
    text-transform: capitalize;
  }

  .service-grid {
    display: grid;
    gap: 1px;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    margin: 0;
    overflow: hidden;
  }

  .service-grid > div {
    background: var(--strip-lift);
    border: 1px solid color-mix(in srgb, var(--rule) 65%, transparent);
    border-radius: var(--r-well);
    min-width: 0;
    padding: var(--space-3);
  }

  .service-grid .wide {
    grid-column: span 2;
  }

  dt {
    color: var(--text-secondary);
    font-size: var(--font-size-small);
  }

  dd {
    font-weight: 600;
    margin: var(--space-1) 0 0;
    min-width: 0;
    overflow-wrap: anywhere;
  }

  dd code {
    font-size: 0.75rem;
  }

  .credential-list {
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .credential-list span {
    background: var(--strip);
    border-radius: var(--r-chip);
    color: var(--text-secondary);
    font-size: 0.75rem;
    padding: 0.15rem 0.4rem;
  }

  .credential-list span.present {
    background: var(--accent-tint);
    color: var(--accent);
  }

  .updated-note {
    justify-self: end;
  }

  .settings-state {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-surface);
    gap: var(--space-3);
    justify-content: center;
    min-height: 12rem;
    padding: var(--space-5);
  }

  .settings-error {
    align-items: flex-start;
    flex-direction: column;
  }

  @media (max-width: 64rem) {
    .runtime-grid,
    .service-grid {
      grid-template-columns: 1fr 1fr;
    }
  }

  @media (max-width: 40rem) {
    .runtime-grid,
    .service-grid {
      grid-template-columns: 1fr;
    }

    .service-grid .wide {
      grid-column: auto;
    }

    .duration-form {
      align-items: stretch;
      display: grid;
      grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
      width: 100%;
    }

    .duration-form .btn {
      grid-column: 1 / -1;
    }

    .duration-input {
      width: 100%;
    }
  }
</style>

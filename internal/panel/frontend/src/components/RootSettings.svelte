<script lang="ts">
  import { untrack } from 'svelte';

  import { formatBytes, formatElapsed, formatLatency } from '../lib/format';
  import type {
    ConfigPatch,
    ConfigValues,
    RootRuntimeSettings,
    RootRuntimeSettingsInput,
  } from '../lib/types';
  import Chip from './Chip.svelte';
  import ConfigEditor from './ConfigEditor.svelte';
  import Icon from './Icon.svelte';
  import InheritControl from './InheritControl.svelte';
  import Plate from './Plate.svelte';
  import RootPageHeader from './RootPageHeader.svelte';

  const DEPLOYMENT_SOURCE = 'the deployment configuration';
  const LOG_LEVELS = [
    { value: 'debug', label: 'Debug' },
    { value: 'info', label: 'Info' },
    { value: 'warn', label: 'Warn' },
    { value: 'error', label: 'Error' },
  ] as const;
  const SESSION_OPTIONS = [{ value: 'custom', label: 'Custom' }] as const;
  const POLL_OPTIONS = [
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
    rootRole,
    fetchSettings,
    updateSettings,
  }: {
    refreshVersion: number;
    rootRole: string;
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

  /* `untrack` because `load` reads `settings` before its first await, to decide
     whether this is a first read or a refresh over settings already on screen.
     That read is inside this effect, and `load` also writes `settings` - with a
     fresh object every time - so each completed read scheduled another one. The
     page asked the server about 1500 times a second. */
  $effect(() => {
    untrack(() => void load(refreshVersion));
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

  function capitalize(value: string): string {
    return value.charAt(0).toUpperCase() + value.slice(1);
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
  <RootPageHeader
    role={rootRole}
    title="Settings"
    subtitle="Runtime behavior and deployment-backed defaults"
  >
    <span class="status-pill"
      ><span class="status-pill-dot live"></span><span class="cap-trim">Changes apply live</span
      ></span
    >
  </RootPageHeader>
  {#if loading && settings === null}
    <div class="settings-state" role="status">Loading runtime settings…</div>
  {:else if settings === null}
    <div class="settings-state settings-error" role="alert">
      <strong>Runtime settings are unavailable</strong>
      <span>{failure}</span>
      <button class="btn" type="button" onclick={() => void load(refreshVersion)}>Try again</button>
    </div>
  {:else}
    {@const current = settings}
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
          Checks reaction changes GitHub does not deliver through webhooks
        </p>
        <div class="session-editor">
          <InheritControl
            label="Reaction sweep interval source"
            source={DEPLOYMENT_SOURCE}
            inheritedValue={current.reaction_poll_interval.deployment_seconds === 0
              ? 'disabled'
              : null}
            inheritedLabel={formatPollInterval(current.reaction_poll_interval.deployment_seconds)}
            value={pollSource === 'default' ? null : pollSource}
            options={POLL_OPTIONS}
            disabled={saving}
            onSelect={(selection) => void selectPollSource(selection)}
            onRestore={() => void selectPollSource('default')}
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
                <span class="select-wrap">
                  <select class="select-input" bind:value={pollUnit} disabled={saving}>
                    <option value="seconds">Seconds</option>
                    <option value="minutes">Minutes</option>
                    <option value="hours">Hours</option>
                  </select>
                  <Icon name="chevron-down" size={14} strokeWidth={2} />
                </span>
              </label>
              <button class="btn btn-signal" type="submit" disabled={saving}>Apply</button>
            </form>
          {/if}
        </div>
      </Plate>

      <Plate label="Log level">
        {#snippet status()}
          <span class="effective-value">Effective: {capitalize(current.log_level.effective)}</span>
        {/snippet}
        <p class="section-intro">Updates the process logger without restarting Smyklot</p>
        <InheritControl
          label="Runtime log level"
          source={DEPLOYMENT_SOURCE}
          inheritedValue={current.log_level.deployment}
          inheritedLabel={capitalize(current.log_level.deployment)}
          value={current.log_level.override}
          options={LOG_LEVELS}
          disabled={saving}
          onSelect={(selection) => void selectLogLevel(selection)}
          onRestore={() => void selectLogLevel('default')}
        />
      </Plate>

      <Plate label="Panel sessions">
        {#snippet status()}
          <span class="effective-value">
            Effective: {formatDuration(current.session_lifetime.effective_seconds)}
          </span>
        {/snippet}
        <p class="section-intro">
          Reductions shorten active sessions; increases apply to new sessions
        </p>
        <div class="session-editor">
          <InheritControl
            label="Session lifetime source"
            source={DEPLOYMENT_SOURCE}
            inheritedLabel={formatDuration(current.session_lifetime.deployment_seconds)}
            value={sessionSource === 'default' ? null : 'custom'}
            options={SESSION_OPTIONS}
            disabled={saving}
            onSelect={(selection) => void selectSessionSource(selection)}
            onRestore={() => void selectSessionSource('default')}
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
                <span class="select-wrap">
                  <select class="select-input" bind:value={sessionUnit} disabled={saving}>
                    <option value="minutes">Minutes</option>
                    <option value="hours">Hours</option>
                    <option value="days">Days</option>
                  </select>
                  <Icon name="chevron-down" size={14} strokeWidth={2} />
                </span>
              </label>
              <button class="btn btn-signal" type="submit" disabled={saving}>Apply</button>
            </form>
          {/if}
        </div>
      </Plate>
    </div>

    <p class="inherit-caption">
      A linked chain inherits from the deployment configuration · the outlined value is what
      inheritance resolves to
    </p>

    <!-- The database has its own plate, and with it the state pill that used to
         stand on this one. A word describing storage over a list of listeners
         and endpoints named neither of them. -->
    <Plate label="Database">
      {#snippet status()}
        <span class="status-pill service-health" data-state={current.service.database.state}
          ><span class="status-pill-dot" aria-hidden="true"></span><span class="cap-trim"
            >{current.service.database.state}</span
          ></span
        >
      {/snippet}
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
    </Plate>

    <Plate label="Service and deployment">
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
            <Chip tone={current.service.credential_presence.webhook ? 'clear' : 'neutral'}
              >Webhook</Chip
            >
            <Chip tone={current.service.credential_presence.app ? 'clear' : 'neutral'}
              >GitHub App</Chip
            >
            <Chip tone={current.service.credential_presence.oauth ? 'clear' : 'neutral'}>OAuth</Chip
            >
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
    border-color: color-mix(in srgb, var(--brand-action) 13%, var(--border-subtle));
  }

  .settings-state,
  .duration-form,
  .credential-list,
  .section-intro,
  .updated-note {
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
    margin: 0;
  }

  .form-error {
    color: var(--stop);
    font-size: var(--font-size-meta);
    margin: 0;
  }

  .source-label,
  .effective-value {
    background: var(--surface-inset);
    border-radius: var(--r-chip);
    /* Self-keyed keyline, same recipe as .chip: the fill alone is near-invisible
       against the plate head. */
    box-shadow: inset 0 0 0 1px color-mix(in srgb, currentcolor 22%, transparent);
    color: var(--text-secondary);
    display: inline-block;
    font: 600 var(--font-size-compact) / 1 var(--sans);
    padding: 0.4rem 0.55rem;
    text-box: trim-both cap alphabetic;
    white-space: nowrap;
  }

  .section-intro {
    margin-bottom: var(--space-3);
  }

  .runtime-grid {
    display: grid;
    gap: var(--space-4);
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .inherit-caption {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    margin: 0;
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

  /* The pill carries the state, marker and word together - the same three states
     the overview's storage value uses. */
  .service-health[data-state='healthy'] {
    background: var(--success-tint);
    color: var(--success);
  }

  .service-health[data-state='degraded'] {
    background: var(--warning-tint);
    color: var(--warning);
  }

  .service-health[data-state='unavailable'] {
    background: var(--danger-tint);
    color: var(--danger);
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

    /* Three plates in two columns would leave a hole; the last one takes the row. */
    .runtime-grid > :global(:last-child) {
      grid-column: 1 / -1;
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

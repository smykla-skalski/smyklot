<script lang="ts">
  import type { PanelApi } from '../lib/api';
  import {
    formatBytes,
    formatElapsed,
    formatLatency,
    formatRelative,
    formatTimestamp,
  } from '../lib/format';
  import type { DependencyState, RootOverview } from '../lib/types';
  import type { ChipTone } from './Chip.svelte';
  import Chip from './Chip.svelte';
  import Icon from './Icon.svelte';
  import PendingCIQueue from './PendingCIQueue.svelte';
  import RootPageHeader from './RootPageHeader.svelte';

  const {
    api,
    refreshVersion,
    rootRole,
    installationsHref,
    elevationsHref,
    failuresHref,
    onOpenInstallations,
    onOpenElevations,
    onOpenFailures,
    onOpenInbox,
  }: {
    api: PanelApi;
    refreshVersion: number;
    rootRole: string;
    installationsHref: string;
    /** Elevations are audited events, so the card opens the audit table. */
    elevationsHref: string;
    /** Delivery health's "View all" - the failure table, not a metric card. */
    failuresHref: string;
    onOpenInstallations: () => void;
    onOpenElevations: () => void;
    onOpenFailures: () => void;
    /** Unread security events ARE the inbox, so that card opens it. */
    onOpenInbox: () => void;
  } = $props();

  /* The hrefs are real addresses - middle-click, Cmd-click and Copy link all
     work - so a plain click is the only one the router takes over. */
  function navigate(event: MouseEvent, open: () => void): void {
    if (
      event.defaultPrevented ||
      event.button !== 0 ||
      event.metaKey ||
      event.ctrlKey ||
      event.shiftKey ||
      event.altKey
    )
      return;
    event.preventDefault();
    open();
  }

  /* The engine names itself. Nothing here maps that name onto behaviour, so a
     third engine is a change inside internal/storage and nowhere else. */
  const DATABASE_MARK: Record<DependencyState, 'check' | 'alert' | 'failure'> = {
    healthy: 'check',
    degraded: 'alert',
    unavailable: 'failure',
  };
  const DATABASE_TONE: Record<DependencyState, ChipTone> = {
    healthy: 'accent',
    degraded: 'warning',
    unavailable: 'stop',
  };
  const DATABASE_WORD: Record<DependencyState, string> = {
    healthy: 'Healthy',
    degraded: 'Degraded',
    unavailable: 'Unreachable',
  };

  let overview = $state<RootOverview | null>(null);
  let loading = $state(true);
  let failure = $state<string | null>(null);
  let now = $state(Date.now());
  let sequence = 0;

  const ownershipTotal = $derived(
    overview === null
      ? 0
      : overview.ownership.fresh +
          overview.ownership.stale +
          overview.ownership.permission_pending +
          overview.ownership.error,
  );
  /* Stale is the neutral state - an ageing snapshot, not a fault - so it colours
     grey in the bar and stays out of this count. Only the warning and danger
     states are things an operator has to act on. */
  const ownershipProblems = $derived(
    overview === null ? 0 : overview.ownership.permission_pending + overview.ownership.error,
  );

  async function load(version = refreshVersion): Promise<void> {
    const current = ++sequence;
    loading = true;
    failure = null;
    try {
      const loaded = await api.fetchRootOverview();
      if (current !== sequence || version !== refreshVersion) return;
      overview = loaded;
      now = Date.now();
    } catch (error) {
      if (current !== sequence || version !== refreshVersion) return;
      failure = error instanceof Error ? error.message : String(error);
    } finally {
      if (current === sequence) loading = false;
    }
  }

  function uptime(seconds: number): string {
    const days = Math.floor(seconds / 86_400);
    const hours = Math.floor((seconds % 86_400) / 3_600);
    const minutes = Math.floor((seconds % 3_600) / 60);
    if (days > 0) return `${days}d ${hours}h`;
    if (hours > 0) return `${hours}h ${minutes}m`;
    return `${minutes}m`;
  }

  function ratio(value: number): number {
    return ownershipTotal === 0 ? 0 : (value / ownershipTotal) * 100;
  }

  /* Against the ceiling, not against what is open: a pool that has opened two
     of sixteen connections is idle, and scaling the track to the two would
     draw it as full. */
  function poolRatio(value: number, max: number): number {
    return max <= 0 ? 0 : Math.min(100, Math.max(0, (value / max) * 100));
  }

  function poolTitle(pool: { in_use: number; idle: number; max: number }): string {
    return `${pool.in_use} in use, ${pool.idle} idle, ${pool.max} maximum`;
  }

  function sentenceCase(text: string): string {
    return text.charAt(0).toUpperCase() + text.slice(1);
  }

  $effect(() => {
    void load(refreshVersion);
  });
</script>

<RootPageHeader
  role={rootRole}
  title="Overview"
  subtitle="Live service, catalog, ownership, and security state"
>
  <span class="status-pill"
    ><span class="status-pill-dot live"></span><span class="cap-trim">WebSocket live</span></span
  >
</RootPageHeader>

<section class="overview" aria-label="Root operational overview">
  {#if failure !== null}
    <div class="overview-error" role="alert">
      <span><Icon name="failure" size={20} /></span>
      <div>
        <strong>Operational state is unavailable</strong>
        <p>{failure}</p>
      </div>
      <button class="btn" type="button" onclick={() => void load(refreshVersion)}>Try again</button>
    </div>
  {:else if loading && overview === null}
    <div class="overview-loading" role="status">
      <span></span><span></span><span></span><span></span>
      <span class="visually-hidden">Loading Root operational overview</span>
    </div>
  {:else if overview !== null}
    <article class="service-card">
      <div class="service-status">
        <!-- A bare check, not a check inside a ring: the tile already is the
             enclosing shape, so the ring drew a second one inside it. -->
        <span class="health-mark"><Icon name="check" size={20} /></span>
        <div>
          <h3>Service health</h3>
          <p>All systems operational</p>
        </div>
      </div>
      <dl>
        <div>
          <dt>Version</dt>
          <dd>{overview.service.version || 'development'}</dd>
        </div>
        <div>
          <dt>Uptime</dt>
          <dd>{uptime(overview.service.uptime_seconds)}</dd>
        </div>
        <div>
          <!-- Which database, not how it is: the marker still carries the
               state, and the card below spells it out. Repeating the word
               "Healthy" here put a third reading of one fact on the screen. -->
          <dt>Database</dt>
          <dd class="storage-state" data-state={overview.service.storage}>
            <span aria-hidden="true">●</span>
            {overview.service.database.engine}
          </dd>
        </div>
        <div>
          <dt>Service</dt>
          <dd>{overview.service.service_host || 'local'}</dd>
        </div>
      </dl>
    </article>

    <!-- The same shape as the card above it, because it answers the same
         question one layer down: what is it, and is it well. The service card
         names the database; this one is how it is doing. -->
    {@const database = overview.service.database}
    <article class="service-card database-card">
      <div class="service-status">
        <span class="health-mark" data-state={database.state}>
          <Icon name={DATABASE_MARK[database.state]} size={20} />
        </span>
        <div>
          <h3>Database</h3>
          <p>
            {database.engine}{database.version === '' ? '' : ` ${database.version}`}
            {#if database.schema_version > 0}
              · schema {database.schema_version}
            {/if}
          </p>
        </div>
        <Chip tone={DATABASE_TONE[database.state]}>{DATABASE_WORD[database.state]}</Chip>
      </div>
      <dl>
        <div>
          <dt>Response</dt>
          <dd>{formatLatency(database.latency_ms)}</dd>
        </div>
        <div>
          <dt>Size</dt>
          <dd>{formatBytes(database.size_bytes)}</dd>
        </div>
        <div class="pool">
          <dt>Connections</dt>
          <dd>{database.connections.in_use} / {database.connections.max}</dd>
          <!-- In use over held over the ceiling. A pool holding four idle
               connections and using one is a service at rest, and a single
               bar could not tell that from one at its limit. -->
          <div class="pool-track" aria-hidden="true" title={poolTitle(database.connections)}>
            <span
              class="pool-open"
              style={`width: ${poolRatio(database.connections.open, database.connections.max)}%`}
            ></span>
            <span
              class="pool-used"
              style={`width: ${poolRatio(database.connections.in_use, database.connections.max)}%`}
            ></span>
          </div>
        </div>
        <div>
          <dt>Waits since start</dt>
          <dd>{database.connections.wait_count}</dd>
        </div>
      </dl>
    </article>

    {#if database.detail !== undefined}
      <p class="database-note" role="status">
        <span class="note-icon warning"><Icon name="alert" size={14} strokeWidth={2} /></span>
        {database.detail}
      </p>
    {:else if database.connections.wait_count > 0}
      <!-- A count that only grows, unlike the sample beside it: the pool may
           look idle now and still have stalled the service an hour ago. -->
      <p class="database-note">
        <span class="note-icon"><Icon name="info" size={14} strokeWidth={2} /></span>
        Callers have waited for a free connection {database.connections.wait_count} times since this service
        started, {formatElapsed(database.connections.wait_ms)} in total
      </p>
    {/if}

    <div class="metric-grid">
      <a
        class="metric-card"
        href={installationsHref}
        onclick={(event) => navigate(event, onOpenInstallations)}
      >
        <span>
          <small>Installations</small>
          <strong>{overview.catalog.installations}</strong>
        </span>
        <span class="metric-chevron"><Icon name="chevron-right" size={14} /></span>
      </a>
      <a
        class="metric-card"
        href={installationsHref}
        onclick={(event) => navigate(event, onOpenInstallations)}
      >
        <span>
          <small>Repositories</small>
          <strong>{overview.catalog.repositories}</strong>
          <em>{overview.catalog.enabled_repositories} enabled</em>
        </span>
        <span class="metric-chevron"><Icon name="chevron-right" size={14} /></span>
      </a>
      <a
        class="metric-card"
        href={elevationsHref}
        onclick={(event) => navigate(event, onOpenElevations)}
      >
        <span>
          <small>Active elevations</small>
          <strong>{overview.active_elevations}</strong>
          <em>15-minute write windows</em>
        </span>
        <span class="metric-chevron"><Icon name="chevron-right" size={14} /></span>
      </a>
      <button
        class:attention={overview.unread_security_events > 0}
        class="metric-card"
        type="button"
        onclick={onOpenInbox}
      >
        <span>
          <small>Unread security events</small>
          <strong>{overview.unread_security_events}</strong>
          <em>Owner notifications</em>
        </span>
        <span class="metric-chevron"><Icon name="chevron-right" size={14} /></span>
      </button>
    </div>

    <PendingCIQueue
      {api}
      queue={overview.pending_ci}
      {now}
      onChanged={() => load(refreshVersion)}
    />

    <div class="overview-columns">
      <article class="overview-panel ownership-panel">
        <header>
          <div>
            <h3>Ownership synchronization</h3>
            <p>
              {ownershipProblems === 0
                ? 'All snapshots trusted'
                : `${ownershipProblems} need attention`}
            </p>
          </div>
          <Chip tone={ownershipProblems === 0 ? 'accent' : 'warning'}>
            {ownershipProblems === 0 ? 'Healthy' : 'Review'}
          </Chip>
        </header>
        <div class="ownership-track" aria-hidden="true">
          {#if overview.ownership.fresh > 0}
            <span class="fresh" style={`width: ${ratio(overview.ownership.fresh)}%`}></span>
          {/if}
          {#if overview.ownership.stale > 0}
            <span class="stale" style={`width: ${ratio(overview.ownership.stale)}%`}></span>
          {/if}
          {#if overview.ownership.permission_pending > 0}
            <span class="pending" style={`width: ${ratio(overview.ownership.permission_pending)}%`}
            ></span>
          {/if}
          {#if overview.ownership.error > 0}
            <span class="failed" style={`width: ${ratio(overview.ownership.error)}%`}></span>
          {/if}
        </div>
        <dl class="ownership-list">
          <div>
            <dt><span class="legend fresh"></span>Fresh</dt>
            <dd>{overview.ownership.fresh}</dd>
          </div>
          <div>
            <dt><span class="legend stale"></span>Stale</dt>
            <dd>{overview.ownership.stale}</dd>
          </div>
          <div>
            <dt><span class="legend pending"></span>Approval needed</dt>
            <dd>{overview.ownership.permission_pending}</dd>
          </div>
          <div>
            <dt><span class="legend failed"></span>Sync failed</dt>
            <dd>{overview.ownership.error}</dd>
          </div>
        </dl>
        {#if overview.ownership.permission_pending > 0}
          <p class="ownership-note">
            <span class="note-icon"><Icon name="alert" size={14} strokeWidth={2} /></span>
            GitHub Members permission approval is blocking Owner synchronization
          </p>
        {/if}
        <a
          class="btn panel-link"
          href={installationsHref}
          onclick={(event) => navigate(event, onOpenInstallations)}
        >
          Review installations <Icon name="chevron-right" size={14} strokeWidth={2} />
        </a>
      </article>

      <article class="overview-panel failures-panel">
        <header>
          <div>
            <h3>Delivery health</h3>
            <p>Recent failures</p>
          </div>
          <a href={failuresHref} onclick={(event) => navigate(event, onOpenFailures)}>
            View all
            <Icon name="chevron-right" size={14} />
          </a>
        </header>
        <div class="failure-list">
          {#each overview.recent_failures as item (item.failure.id)}
            <div class="failure-item">
              <!-- The kind is carried by the glyph and its colour, so a retry
                   reads as a retry before the chip on the right is reached. -->
              <span class:retryable={item.failure.retryable} class="failure-mark">
                <Icon name={item.failure.retryable ? 'refresh' : 'failure'} size={14} />
              </span>
              <div>
                <strong>{sentenceCase(item.failure.reason)}</strong>
                <!-- Inline, not a block: the mock leaves this line in the
                     panel's own line box, and a block would size it to its own
                     shorter leading and pull the entry up 4px. -->
                <span class="failure-meta">
                  {item.installation.display_name} ·
                  <code>{item.failure.repository_full_name}</code> ·
                  <time
                    datetime={item.failure.occurred_at}
                    title={formatTimestamp(item.failure.occurred_at)}
                    >{formatRelative(item.failure.occurred_at, now)}</time
                  >
                </span>
              </div>
              <span class="failure-kind">
                <Chip tone={item.failure.retryable ? 'warning' : 'stop'}>
                  {item.failure.retryable ? 'Retryable' : 'Permanent'}
                </Chip>
              </span>
            </div>
          {:else}
            <div class="no-failures">
              <span><Icon name="success" size={20} /></span>
              <div>
                <strong>No retained failures</strong>
                <p>Recent deliveries are healthy</p>
              </div>
            </div>
          {/each}
        </div>
      </article>
    </div>
  {/if}
</section>

<style>
  /* One 12px rhythm down the page: the service plate, the stat row, and the
     two-column block sit the same distance apart as the cards inside them. */
  .overview {
    display: grid;
    gap: var(--space-3);
  }

  .service-card,
  .service-status,
  .metric-card,
  .overview-panel > header,
  .failure-item,
  .no-failures,
  .ownership-note,
  .database-note {
    align-items: center;
    display: flex;
  }

  .service-status p,
  .overview-panel header p,
  .no-failures p,
  .ownership-note {
    color: var(--text-secondary);
    margin: 0;
  }

  .service-card,
  .overview-panel,
  .metric-card,
  .overview-error {
    background: var(--surface-base);
    border: 1px solid color-mix(in srgb, var(--brand-action) 13%, var(--border-subtle));
    border-radius: var(--radius-surface);
    box-shadow: var(--shadow-plate);
  }

  .service-card {
    gap: var(--space-6);
    justify-content: space-between;
    padding: var(--space-5);
  }

  /* The mark and the copy are siblings on the card's own 24px rhythm - the
     mock has no tighter inner gap here. */
  .service-status {
    gap: var(--space-6);
  }

  .service-status h3,
  .overview-panel h3 {
    font-size: var(--font-size-title);
    letter-spacing: -0.025em;
    line-height: 1.3;
    margin: 0;
  }

  /* Card heads lead with the title; the changing detail sits under it as a
     compact sub-line, so scanning the page reads what each card IS first. */
  .service-status div > p,
  .overview-panel header div > p {
    font-size: var(--font-size-compact);
    line-height: 1.4;
    margin-top: 0.15rem;
  }

  .health-mark,
  .no-failures > span {
    align-items: center;
    background: var(--accent-tint);
    border-radius: var(--radius-control);
    color: var(--accent);
    display: inline-flex;
    flex: 0 0 auto;
    justify-content: center;
  }

  .health-mark {
    height: 3rem;
    width: 3rem;
  }

  /* The service card's own mark carries no state and keeps the accent above.
     Only the database reports one, so only it repaints. */
  .health-mark[data-state='degraded'] {
    background: var(--warning-tint);
    color: var(--warning);
  }

  .health-mark[data-state='unavailable'] {
    background: var(--danger-tint);
    color: var(--danger);
  }

  .service-card dl {
    display: grid;
    gap: var(--space-3) var(--space-6);
    grid-template-columns: repeat(4, minmax(5.5rem, auto));
    margin: 0;
  }

  .service-card dt,
  .metric-card small {
    color: var(--text-muted);
    font: 700 var(--font-size-micro) / 1.3 var(--sans);
    letter-spacing: 0.05em;
    text-transform: uppercase;
  }

  /* A plain block, not a flex row: the storage marker is a bullet inside the
     mono run, so it takes the run's own advance and sits on its baseline. A
     flex gap put it a pixel out from every other value in the row. */
  .service-card dd {
    font: 600 var(--font-size-compact) / 1.5 var(--mono);
    margin: 0.15rem 0 0;
  }

  /* The whole value carries the state colour, marker and word together. */
  .storage-state[data-state='healthy'] {
    color: var(--success);
  }

  .storage-state[data-state='degraded'] {
    color: var(--warning);
  }

  .storage-state[data-state='unavailable'] {
    color: var(--stop);
  }

  /* Wider than the service card's cells: this row carries a value with a track
     under it, and 5.5rem left the track too short to read a proportion off. */
  .database-card dl {
    grid-template-columns: repeat(4, minmax(7rem, auto));
  }

  .pool {
    align-content: start;
  }

  .pool-track {
    background: var(--surface-inset);
    border-radius: 999px;
    height: 0.25rem;
    margin-top: 0.4rem;
    overflow: hidden;
    position: relative;
  }

  /* Stacked rather than laid side by side, so the shorter run reads as a part
     of the longer one: connections in use are a subset of those held open. */
  .pool-track > span {
    border-radius: 999px;
    bottom: 0;
    left: 0;
    position: absolute;
    top: 0;
  }

  .pool-open {
    background: var(--border-strong);
  }

  .pool-used {
    background: var(--accent);
  }

  .metric-grid {
    display: grid;
    gap: var(--space-3);
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }

  /* No icon tile: the mock's stat card is a label, a number, and a caption,
     with the chevron floated off in the padding. An icon pushed the copy
     right and left the number nowhere to breathe. */
  .metric-card {
    /* Stretch, not the shared `center`: this one is a COLUMN, so the inherited
       cross-axis centring pushed the whole label/number/caption block into the
       middle of the card instead of leaving it on the padding edge. */
    align-items: stretch;
    color: var(--text-primary);
    display: flex;
    flex-direction: column;
    justify-content: center;
    min-height: 6.5rem;
    padding: var(--space-4) 2rem var(--space-4) var(--space-4);
    position: relative;
    text-decoration: none;
  }

  button.metric-card {
    border: 1px solid color-mix(in srgb, var(--brand-action) 13%, var(--border-subtle));
    cursor: pointer;
    font: inherit;
    text-align: left;
  }

  a.metric-card:hover,
  button.metric-card:hover {
    border-color: color-mix(in srgb, var(--brand-action) 34%, var(--border-subtle));
    transform: translateY(-1px);
  }

  a.metric-card:active,
  button.metric-card:active {
    transform: scale(var(--press-scale-surface));
  }

  .metric-card.attention {
    background: color-mix(in srgb, var(--warning) 5%, var(--surface-base));
    border-color: color-mix(in srgb, var(--warning) 30%, var(--border-subtle));
  }

  .metric-card > span:first-of-type {
    display: grid;
    gap: 0.45rem;
  }

  .metric-card strong {
    font: 700 1.55rem/1 var(--mono);
  }

  .metric-card em {
    color: var(--text-secondary);
    font-size: var(--font-size-micro);
    font-style: normal;
    line-height: 1.3;
  }

  .metric-chevron {
    color: var(--text-muted);
    display: inline-flex;
    position: absolute;
    right: 0.7rem;
    top: 50%;
    transform: translateY(-50%);
  }

  /* The two cards stand the same height, as the approved pair does: side by
     side they read as one band, and a short card next to a tall one leaves a
     step in the bottom edge. Their content stays top-aligned inside. */
  .overview-columns {
    display: grid;
    gap: var(--space-3);
    grid-template-columns: minmax(18rem, 0.82fr) minmax(25rem, 1.18fr);
  }

  .overview-panel {
    min-width: 0;
    padding: var(--space-5);
  }

  .overview-panel > header {
    align-items: flex-start;
    justify-content: space-between;
  }

  /* Both badges are Chips now rather than two hand-rolled copies of the chip
     recipe: the tint, the keyline, the padding and the label's trim all live in
     one place, so a fix to chips reaches these without being ported. */
  .failure-kind {
    flex: none;
  }

  .ownership-track {
    display: flex;
    gap: 3px;
    height: 0.55rem;
    margin: var(--space-5) 0 var(--space-4);
  }

  .ownership-track > span {
    border-radius: 999px;
    min-width: 6px;
  }

  .fresh {
    background: var(--success);
  }

  .stale {
    background: var(--border-strong);
  }

  .pending {
    background: var(--warning);
  }

  .failed {
    background: var(--danger);
  }

  .ownership-list {
    display: grid;
    gap: var(--space-2);
    margin: 0;
  }

  .ownership-list > div {
    align-items: center;
    display: flex;
    justify-content: space-between;
  }

  .ownership-list dt {
    align-items: center;
    color: var(--text-secondary);
    display: flex;
    font-size: var(--font-size-compact);
    gap: 0.55rem;
    line-height: 1;
  }

  .ownership-list dd {
    font: 650 var(--font-size-compact) / 1 var(--mono);
    margin: 0;
  }

  /* 8px dot, 8.8px gap - the swatch and the space beside it had been swapped,
     which read as a slightly fat dot crowding its label. */
  .legend {
    border-radius: 50%;
    height: 0.5rem;
    width: 0.5rem;
  }

  .ownership-note,
  .database-note {
    align-items: flex-start;
    background: var(--surface-inset);
    /* Dashed: the blocker is a pending state, not a settled surface. */
    border: 1px dashed var(--control-border);
    border-radius: var(--radius-control);
    font-size: var(--font-size-compact);
    gap: var(--space-2);
    line-height: 1.5;
    padding: 0.5rem 0.75rem;
  }

  .ownership-note {
    margin-top: var(--space-4);
  }

  /* Its own row on the page's 12px rhythm, rather than inside the card: the
     reason is prose that wraps, and a card sized for four short values had
     nowhere to put a sentence. */
  .database-note {
    margin: 0;
  }

  /* The wait note reports a fact, not a fault, so only the reason a database
     gave for refusing to describe itself takes the warning colour. */
  .database-note .note-icon {
    color: var(--text-muted);
  }

  .database-note .note-icon.warning {
    color: var(--warning);
  }

  .note-icon {
    color: var(--warning);
    display: grid;
    flex: none;
    /* One line-box tall, so the glyph centers on the first line of a wrapped note. */
    height: 1.125rem;
    place-items: center;
    width: 1.125rem;
  }

  .panel-link {
    align-self: flex-start;
    margin-top: var(--space-3);
  }

  .overview-panel header > a {
    align-items: center;
    color: var(--accent);
    display: inline-flex;
    font: 650 var(--font-size-compact) / 1 var(--sans);
    gap: 0.2rem;
    text-decoration: none;
  }

  .failure-list {
    display: grid;
    margin-top: var(--space-3);
  }

  /* Top-aligned, so a wrapped reason keeps its icon beside the first line
     rather than drifting to the middle of the block. */
  .failure-item {
    align-items: flex-start;
    gap: var(--space-3);
    padding: var(--space-3) 0;
  }

  .failure-item + .failure-item {
    border-top: 1px solid var(--border-subtle);
  }

  .failure-item > div {
    flex: 1;
    min-width: 0;
  }

  .failure-item strong {
    display: block;
    font-size: var(--font-size-meta);
    line-height: 1.4;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* Installation, repository, and age share one line. Stacked on three, the
     entry stood half again as tall as the mock's and the panel ran past the
     ownership card beside it. */
  .failure-meta {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    line-height: 1.5;
  }

  .failure-item code,
  .failure-item time {
    color: inherit;
    font-size: inherit;
  }

  /* A bare 18px icon slot, not a filled tile. Every entry in this list is a
     failure, so tinting a square behind each one only added weight the mock
     spends on the reason text instead. */
  .failure-mark {
    color: var(--stop);
    display: grid;
    flex: none;
    height: 1.125rem;
    /* The glyph's optical centre sits a hair below its box; the mock nudges it
       back onto the first line's cap. */
    margin-top: -0.25px;
    place-items: center;
    width: 1.125rem;
  }

  .failure-mark.retryable {
    color: var(--warning);
  }

  .no-failures > span {
    background: var(--stop-tint);
    color: var(--stop);
    height: 2rem;
    width: 2rem;
  }

  .no-failures {
    gap: var(--space-3);
    min-height: 8rem;
    justify-content: center;
  }

  .no-failures > span {
    background: var(--accent-tint);
    color: var(--accent);
  }

  .overview-error {
    align-items: center;
    display: grid;
    gap: var(--space-3);
    grid-template-columns: auto minmax(0, 1fr) auto;
    padding: var(--space-5);
  }

  .overview-error p {
    color: var(--text-secondary);
    margin: var(--space-1) 0 0;
  }

  .overview-loading {
    display: grid;
    gap: var(--space-3);
    grid-template-columns: repeat(4, 1fr);
  }

  .overview-loading span:not(.visually-hidden) {
    animation: pulse 1.35s ease-in-out infinite alternate;
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-surface);
    min-height: 7rem;
  }

  @keyframes pulse {
    to {
      opacity: 0.55;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .overview-loading span:not(.visually-hidden) {
      animation: none;
    }
    a.metric-card {
      transition: none;
    }
  }

  @media (max-width: 68rem) {
    .metric-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
    .overview-columns {
      grid-template-columns: 1fr;
    }
    .service-card {
      align-items: start;
      flex-direction: column;
    }
    .service-card dl {
      width: 100%;
    }
  }

  @media (max-width: 42rem) {
    .metric-grid,
    .service-card dl,
    .overview-loading {
      grid-template-columns: 1fr;
    }
    .failure-item {
      align-items: start;
    }
    .failure-kind {
      display: none;
    }
  }
</style>

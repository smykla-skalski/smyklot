<script lang="ts">
  import type { PanelApi } from '../lib/api';
  import { formatRelative, formatTimestamp } from '../lib/format';
  import type { RootOverview } from '../lib/types';
  import Icon from './Icon.svelte';

  const {
    api,
    refreshVersion,
    installationsHref,
    failuresHref,
  }: {
    api: PanelApi;
    refreshVersion: number;
    installationsHref: string;
    failuresHref: string;
  } = $props();

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
  const ownershipProblems = $derived(
    overview === null
      ? 0
      : overview.ownership.stale + overview.ownership.permission_pending + overview.ownership.error,
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

  $effect(() => {
    void load(refreshVersion);
  });
</script>

<section class="overview" aria-label="Root operational overview">
  <div class="overview-actions">
    <p>Live service, catalog, ownership, and security state</p>
    <span class="live-state"><span aria-hidden="true"></span> WebSocket live</span>
  </div>

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
        <span class="health-mark"><Icon name="success" size={22} /></span>
        <div>
          <p>Service health</p>
          <h3>All systems operational</h3>
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
          <dt>Storage</dt>
          <dd><span class="status-dot"></span>{overview.service.storage}</dd>
        </div>
        <div>
          <dt>Service</dt>
          <dd>{overview.service.service_host || 'local'}</dd>
        </div>
      </dl>
    </article>

    <div class="metric-grid">
      <a class="metric-card" href={installationsHref}>
        <span class="metric-icon"><Icon name="organization" size={19} /></span>
        <span>
          <small>Installations</small>
          <strong>{overview.catalog.installations}</strong>
        </span>
        <Icon name="chevron-right" size={17} />
      </a>
      <a class="metric-card" href={installationsHref}>
        <span class="metric-icon"><Icon name="repositories" size={19} /></span>
        <span>
          <small>Repositories</small>
          <strong>{overview.catalog.repositories}</strong>
          <em>{overview.catalog.enabled_repositories} enabled</em>
        </span>
        <Icon name="chevron-right" size={17} />
      </a>
      <div class:attention={overview.active_elevations > 0} class="metric-card">
        <span class="metric-icon"><Icon name="owner" size={19} /></span>
        <span>
          <small>Active elevations</small>
          <strong>{overview.active_elevations}</strong>
          <em>15-minute write windows</em>
        </span>
      </div>
      <div class:attention={overview.unread_security_events > 0} class="metric-card">
        <span class="metric-icon"><Icon name="notifications" size={19} /></span>
        <span>
          <small>Unread security events</small>
          <strong>{overview.unread_security_events}</strong>
          <em>Owner notifications</em>
        </span>
      </div>
    </div>

    <div class="overview-columns">
      <article class="overview-panel ownership-panel">
        <header>
          <div>
            <p>Ownership synchronization</p>
            <h3>
              {ownershipProblems === 0
                ? 'All snapshots trusted'
                : `${ownershipProblems} need attention`}
            </h3>
          </div>
          <span class:warning={ownershipProblems > 0} class="health-label">
            {ownershipProblems === 0 ? 'Healthy' : 'Review'}
          </span>
        </header>
        <div class="ownership-track" aria-hidden="true">
          <span class="fresh" style={`width: ${ratio(overview.ownership.fresh)}%`}></span>
          <span class="stale" style={`width: ${ratio(overview.ownership.stale)}%`}></span>
          <span class="pending" style={`width: ${ratio(overview.ownership.permission_pending)}%`}
          ></span>
          <span class="failed" style={`width: ${ratio(overview.ownership.error)}%`}></span>
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
            <dt><span class="legend pending"></span>Permission approval</dt>
            <dd>{overview.ownership.permission_pending}</dd>
          </div>
          <div>
            <dt><span class="legend failed"></span>Sync error</dt>
            <dd>{overview.ownership.error}</dd>
          </div>
        </dl>
        {#if overview.ownership.permission_pending > 0}
          <p class="ownership-note">
            <Icon name="warning" size={15} /> GitHub Members permission approval is blocking Owner synchronization.
          </p>
        {/if}
        <a class="btn btn-row panel-link" href={installationsHref}>
          Review installations <Icon name="chevron-right" size={15} />
        </a>
      </article>

      <article class="overview-panel failures-panel">
        <header>
          <div>
            <p>Delivery health</p>
            <h3>Recent failures</h3>
          </div>
          <a href={failuresHref}>View all</a>
        </header>
        <div class="failure-list">
          {#each overview.recent_failures as item (item.failure.id)}
            <div class="failure-item">
              <span class="failure-mark"><Icon name="failure" size={16} /></span>
              <div>
                <strong>{item.failure.reason}</strong>
                <p>
                  {item.installation.display_name} ·
                  <code>{item.failure.repository_full_name}</code>
                </p>
                <time
                  datetime={item.failure.occurred_at}
                  title={formatTimestamp(item.failure.occurred_at)}
                  >{formatRelative(item.failure.occurred_at, now)}</time
                >
              </div>
              <span class:retryable={item.failure.retryable} class="failure-kind">
                {item.failure.retryable ? 'Retryable' : 'Permanent'}
              </span>
            </div>
          {:else}
            <div class="no-failures">
              <span><Icon name="success" size={20} /></span>
              <div>
                <strong>No retained failures</strong>
                <p>Recent deliveries are healthy.</p>
              </div>
            </div>
          {/each}
        </div>
      </article>
    </div>
  {/if}
</section>

<style>
  .overview {
    display: grid;
    gap: var(--space-4);
  }

  .overview-actions,
  .service-card,
  .service-status,
  .metric-card,
  .overview-panel > header,
  .failure-item,
  .no-failures,
  .ownership-note {
    align-items: center;
    display: flex;
  }

  .overview-actions {
    justify-content: space-between;
  }

  .live-state {
    align-items: center;
    color: var(--admin);
    display: inline-flex;
    font-size: var(--font-size-compact);
    font-weight: 650;
    gap: var(--space-2);
  }

  .live-state > span {
    background: currentColor;
    border-radius: 50%;
    box-shadow: 0 0 0 0.22rem color-mix(in srgb, currentColor 13%, transparent);
    height: 0.45rem;
    width: 0.45rem;
  }

  .overview-actions p,
  .service-status p,
  .overview-panel header p,
  .failure-item p,
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
    border: 1px solid color-mix(in srgb, #8b5cf6 13%, var(--border-subtle));
    border-radius: var(--radius-surface);
    box-shadow: var(--shadow-plate);
  }

  .service-card {
    gap: var(--space-6);
    justify-content: space-between;
    padding: var(--space-5);
  }

  .service-status {
    gap: var(--space-3);
  }

  .service-status h3,
  .overview-panel h3 {
    font-size: var(--font-size-title);
    letter-spacing: -0.025em;
    margin: var(--space-1) 0 0;
  }

  .health-mark,
  .metric-icon,
  .failure-mark,
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

  .service-card dl {
    display: grid;
    gap: var(--space-3) var(--space-6);
    grid-template-columns: repeat(4, minmax(5.5rem, auto));
    margin: 0;
  }

  .service-card dt,
  .metric-card small {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
    font-weight: 700;
    letter-spacing: 0.05em;
    text-transform: uppercase;
  }

  .service-card dd {
    align-items: center;
    display: flex;
    font: 600 var(--font-size-compact) / 1.4 var(--mono);
    gap: var(--space-1);
    margin: var(--space-1) 0 0;
  }

  .status-dot {
    background: var(--accent);
    border-radius: 50%;
    height: 0.45rem;
    width: 0.45rem;
  }

  .metric-grid {
    display: grid;
    gap: var(--space-3);
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }

  .metric-card {
    color: var(--text-primary);
    gap: var(--space-3);
    min-height: 6.5rem;
    padding: var(--space-4);
    text-decoration: none;
  }

  a.metric-card:hover {
    border-color: color-mix(in srgb, #8b5cf6 34%, var(--border-subtle));
    transform: translateY(-1px);
  }

  a.metric-card:active {
    transform: translateY(1px) scale(0.995);
  }

  .metric-card.attention {
    background: color-mix(in srgb, var(--warning) 5%, var(--surface-base));
    border-color: color-mix(in srgb, var(--warning) 30%, var(--border-subtle));
  }

  .metric-card > span:nth-child(2) {
    display: grid;
    flex: 1;
    gap: var(--space-1);
  }

  .metric-card strong {
    font: 700 1.55rem/1 var(--mono);
  }

  .metric-card em {
    color: var(--text-secondary);
    font-size: var(--font-size-micro);
    font-style: normal;
  }

  .metric-icon {
    height: 2.4rem;
    width: 2.4rem;
  }

  .overview-columns {
    display: grid;
    gap: var(--space-4);
    grid-template-columns: minmax(18rem, 0.82fr) minmax(25rem, 1.18fr);
  }

  .overview-panel {
    min-width: 0;
    padding: var(--space-5);
  }

  .overview-panel > header {
    justify-content: space-between;
  }

  .health-label,
  .failure-kind {
    background: var(--accent-tint);
    border: 1px solid color-mix(in srgb, var(--accent) 22%, var(--border-subtle));
    border-radius: var(--radius-control);
    color: var(--accent);
    font-size: var(--font-size-micro);
    font-weight: 700;
    padding: 0.35rem 0.5rem;
  }

  .health-label.warning,
  .failure-kind:not(.retryable) {
    background: var(--warning-tint);
    border-color: color-mix(in srgb, var(--warning) 30%, var(--border-subtle));
    color: var(--warning);
  }

  .ownership-track {
    background: var(--surface-inset);
    border-radius: 999px;
    display: flex;
    height: 0.55rem;
    margin: var(--space-5) 0 var(--space-4);
    overflow: hidden;
  }

  .fresh,
  .status-dot {
    background: var(--accent);
  }

  .stale {
    background: var(--text-muted);
  }

  .pending {
    background: var(--warning);
  }

  .failed {
    background: var(--stop);
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
    gap: var(--space-2);
  }

  .ownership-list dd {
    font: 650 var(--font-size-compact) / 1 var(--mono);
    margin: 0;
  }

  .legend {
    border-radius: 50%;
    height: 0.55rem;
    width: 0.55rem;
  }

  .ownership-note {
    background: color-mix(in srgb, var(--warning) 6%, var(--surface-inset));
    border-radius: var(--radius-control);
    font-size: var(--font-size-compact);
    gap: var(--space-2);
    margin-top: var(--space-4);
    padding: var(--space-3);
  }

  .panel-link {
    margin-top: var(--space-4);
  }

  .overview-panel header > a {
    color: var(--accent);
    font-size: var(--font-size-compact);
    font-weight: 650;
  }

  .failure-list {
    display: grid;
    margin-top: var(--space-3);
  }

  .failure-item {
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
    font-size: var(--font-size-compact);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .failure-item p,
  .failure-item time {
    font-size: var(--font-size-micro);
  }

  .failure-item code {
    font-size: inherit;
  }

  .failure-item time {
    color: var(--text-muted);
  }

  .failure-mark,
  .no-failures > span {
    background: var(--stop-tint);
    color: var(--stop);
    height: 2rem;
    width: 2rem;
  }

  .failure-kind.retryable {
    background: color-mix(in srgb, #8b5cf6 8%, var(--surface-base));
    border-color: color-mix(in srgb, #8b5cf6 26%, var(--border-subtle));
    color: #6d54bd;
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
    animation: pulse 850ms ease-in-out infinite alternate;
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
    .overview-actions {
      align-items: start;
      gap: var(--space-3);
    }
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

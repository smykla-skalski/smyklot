<script lang="ts">
  import { formatRelative, formatUntil } from '../format';
  import {
    SYNC_KINDS,
    type SyncConfig,
    type SyncKind,
    type SyncPlan,
    type SyncStatus,
  } from '../types';
  import { SYNC_SECTION_LABELS, type SyncSection } from '../routes';
  import { repositorySyncHealth, syncIssues } from '../sync-health';
  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import Icon from './Icon.svelte';
  import PageHeader from './PageHeader.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import Switch from './Switch.svelte';
  import SearchField from './SearchField.svelte';

  const {
    status,
    plan,
    configs,
    nowMs,
    repositories = null,
    sectionHref,
    onOpenSection,
    onToggleKind,
    dirtyControls = [],
    readOnly = false,
    canControl = false,
    busy = false,
    onCheck = () => {},
    onDetails = () => {},
    repositoryHref = null,
    permissionsHref = null,
    queueHref = null,
    savedConfigs = configs,
  }: {
    savedConfigs?: Partial<Record<SyncKind, SyncConfig>>;
    permissionsHref?: string | null;
    queueHref?: string | null;
    status: SyncStatus;
    plan: SyncPlan | null;
    configs: Partial<Record<SyncKind, SyncConfig>>;
    nowMs: number;
    repositories?: number | null;
    sectionHref: (section: SyncSection) => string;
    onOpenSection: (section: SyncSection) => void;
    onToggleKind: (kind: SyncKind, enabled: boolean) => void;
    dirtyControls?: readonly string[];
    readOnly?: boolean;
    canControl?: boolean;
    busy?: boolean;
    onCheck?: () => void;
    onDetails?: (trigger: HTMLElement) => void;
    repositoryHref?: ((repository: string) => string) | null;
  } = $props();

  let filter = $state('all');
  let openRepository = $state<string | null>(null);
  let search = $state('');
  let showAll = $state(false);
  const issues = $derived(syncIssues(status, plan, savedConfigs));
  const rows = $derived(status.repositories);
  const activeKinds = $derived(SYNC_KINDS.filter((kind) => savedConfigs[kind]?.enabled));
  const counts = $derived({
    settled: rows.filter((row) => repositorySyncHealth(row) === 'settled').length,
    syncing: rows.filter((row) => repositorySyncHealth(row) === 'syncing').length,
    blocked: rows.filter((row) => repositorySyncHealth(row) === 'blocked').length,
    paused: rows.filter((row) => repositorySyncHealth(row) === 'paused').length,
  });
  const options = $derived([
    { value: 'all', label: 'All', badge: rows.length },
    { value: 'blocked', label: 'Blocked', badge: counts.blocked },
    { value: 'syncing', label: 'Syncing', badge: counts.syncing },
    { value: 'settled', label: 'Up to date', badge: counts.settled },
    { value: 'paused', label: 'Paused', badge: counts.paused },
  ]);
  const matching = $derived(
    rows
      .filter(
        (row) =>
          (filter === 'all' || repositorySyncHealth(row) === filter) &&
          row.repository.toLowerCase().includes(search.toLowerCase()),
      )
      .toSorted((a, b) => {
        const order = { blocked: 0, syncing: 1, settled: 2, paused: 3 };
        return (
          order[repositorySyncHealth(a)] - order[repositorySyncHealth(b)] ||
          a.repository.localeCompare(b.repository)
        );
      }),
  );
  const visible = $derived(showAll ? matching : matching.slice(0, 8));
  const ongoing = $derived(plan !== null && ['approved', 'applying'].includes(plan.state));
  const queueBlocked = $derived(plan?.queue_item?.state === 'blocked');
  const scheduled = $derived(plan?.queue_item?.eligible_at);
  const applied = $derived(
    plan?.actions.filter((action) => action.state === 'applied').length ?? 0,
  );
  const healthWords = {
    blocked: 'Blocked',
    syncing: 'Syncing',
    settled: 'Up to date',
    paused: 'Paused',
  };
  const cellWords = {
    refused: 'Blocked',
    pending: 'Syncing',
    in_step: 'Up to date',
    off: 'Paused',
  };
  const kindIcons = {
    labels: 'tag',
    settings: 'sliders',
    rulesets: 'branch',
    files: 'file',
  } as const;

  function kindSummary(kind: SyncKind): string {
    const config = configs[kind];
    if (!config) return 'Loading configuration';
    if (config.unreadable) return 'Configuration needs a fix';
    if (kind === 'labels') return `${config.labels.length} shared labels`;
    if (kind === 'settings') return `${Object.keys(config.document).length} managed options`;
    const list = config.document[kind];
    return `${Array.isArray(list) ? list.length : 0} ${kind === 'files' ? 'shared templates' : 'rulesets'}`;
  }
  function open(event: MouseEvent, section: SyncSection): void {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey)
      return;
    event.preventDefault();
    onOpenSection(section);
  }
</script>

<!--
@component
The health of automatic sync. Routine work reports progress without asking for a
review. Only blockers enter Needs attention; their reasons and recovery links stay
beside the affected repository. Change details open over this view, preserving context.
-->

<section class="sync-home view-frame" aria-labelledby="sync-overview-heading">
  <PageHeader
    id="sync-overview-heading"
    section="Sync"
    title="Sync status"
    description="Your shared configuration, kept in step automatically"
  >
    {#snippet actions()}
      {#if canControl && !ongoing}<Button disabled={busy} onclick={onCheck}
          >{busy ? 'Checking…' : 'Check now'}</Button
        >{/if}
    {/snippet}
  </PageHeader>

  <Card>
    <div class="card-head verdict-head">
      <h2 class="card-title">
        {activeKinds.length === 0 || queueBlocked ? 'Sync is paused' : 'Automatic sync is on'}
      </h2>
      <span class="card-note band-trim">
        {#if activeKinds.length === 0}Enable a configuration below to start syncing
        {:else if queueBlocked}Resume automatic sync in Queue to continue
        {:else if issues.length > 0}{issues.length}
          {issues.length === 1 ? 'issue needs' : 'issues need'} your attention · other changes continue
          automatically
        {:else}Saved configuration is kept in sync · no action needed{/if}
      </span>
    </div>
    <div class="sync-totals" aria-label="Repository sync summary">
      {#each ['settled', 'syncing', 'blocked', 'paused'] as state (state)}
        <div class="sync-total" data-health={state}>
          <strong class="band-trim">{counts[state as keyof typeof counts]}</strong>
          <span class="band-trim"
            >{healthWords[state as keyof typeof healthWords].toLowerCase()}</span
          >
        </div>
      {/each}
    </div>
    <div class="sync-freshness">
      <span class="band-trim"
        >Last checked {formatRelative(status.checked_at, nowMs)} · {repositories ?? rows.length} repositories</span
      >
      {#if savedConfigs.files?.enabled}<span class="band-trim"
          >Shared files arrive as pull requests</span
        >{/if}
    </div>
    {#if ongoing}
      <div class="sync-progress">
        <div class="object-main">
          <span class="object-name"
            >{queueBlocked
              ? 'Queued changes are on hold'
              : plan?.state === 'applying'
                ? `Syncing · ${applied} of ${plan?.actions.length} changes applied`
                : 'Changes queued for sync'}</span
          >
          <span class="object-sum">
            {#if queueBlocked}Your saved configuration is waiting for the queue policy to resume
            {:else if plan?.queue_item?.state === 'retrying'}A temporary error interrupted sync ·
              Smyklot retries automatically
            {:else if scheduled && Date.parse(scheduled) > nowMs}Runs {formatUntil(
                scheduled,
                nowMs,
              )} within configured hours
            {:else}Sync continues in the background{/if}
          </span>
        </div>
        <Button tone="quiet" onclick={(event) => onDetails(event.currentTarget)}
          >View changes</Button
        >
      </div>
    {/if}
  </Card>

  {#if issues.length > 0}
    <Card labelledby="sync-attention-label">
      <div class="card-head">
        <h2 class="card-title" id="sync-attention-label">Needs attention</h2>
        <span class="card-note band-trim"
          >{issues.length} {issues.length === 1 ? 'issue' : 'issues'}</span
        >
      </div>
      <ul class="object-list">
        {#each issues as issue (issue.id)}
          <li>
            <div class="object-row sync-issue-row">
              <div class="object-main">
                <h3 class="object-name">{issue.title}</h3>
                <span class="object-sum"
                  >{issue.kind ? `${SYNC_SECTION_LABELS[issue.kind]} · ` : ''}{issue.detail}</span
                >
              </div>
              <div class="object-side">
                {#if issue.id === 'system:legacy-approval'}
                  <Button row onclick={(event) => onDetails(event.currentTarget)}
                    >Review changes</Button
                  >
                {:else if issue.queue && queueHref}
                  <Button row href={queueHref}>Open Queue</Button>
                {:else if issue.permission && permissionsHref}
                  <Button row href={permissionsHref} target="_blank" rel="noopener noreferrer"
                    >Review GitHub access</Button
                  >
                {:else if issue.repository && repositoryHref}
                  <Button row href={repositoryHref(issue.repository)}>Open repository</Button>
                {:else if issue.kind}
                  <Button
                    row
                    href={sectionHref(issue.kind)}
                    onclick={(event) => open(event, issue.kind!)}>Edit configuration</Button
                  >
                {/if}
              </div>
            </div>
          </li>
        {/each}
      </ul>
    </Card>
  {/if}

  <Card labelledby="sync-repositories-heading">
    <div class="card-head">
      <h2 class="card-title" id="sync-repositories-heading">Repositories</h2>
      <span class="card-note band-trim">
        {visible.length === matching.length
          ? matching.length
          : `${visible.length} of ${matching.length}`}
      </span>
      {#if matching.length > 8}
        <Button
          row
          tone="quiet"
          aria-label={showAll
            ? 'Show fewer repositories'
            : `Show all ${matching.length} repositories`}
          aria-expanded={showAll}
          aria-controls="sync-repository-list"
          onclick={() => (showAll = !showAll)}
        >
          {#snippet trailing()}<Icon
              name={showAll ? 'chevron-up' : 'chevron-down'}
              size="xs"
            />{/snippet}
          {showAll ? 'Show fewer' : 'Show all'}
        </Button>
      {/if}
    </div>
    <div class="filter-bar">
      <SearchField
        label="Find a syncing repository"
        placeholder="Find a repository"
        value={search}
        onInput={(value) => {
          search = value;
          showAll = false;
        }}
      />
      <div class="sync-filter-options">
        <SegmentedControl
          name="sync-health"
          label="Repository sync state"
          {options}
          value={filter}
          onSelect={(value) => {
            filter = value;
            showAll = false;
          }}
        />
      </div>
    </div>
    {#if matching.length === 0}
      <div class="state-panel sync-empty">
        <span
          ><strong
            >{rows.length === 0 ? 'No repositories to sync yet.' : 'No repositories match.'}</strong
          >
          {rows.length === 0
            ? 'Repositories will appear here after the workspace inventory refreshes'
            : 'Try another name or sync state'}</span
        >
        {#if search || filter !== 'all'}<Button
            tone="quiet"
            onclick={() => {
              search = '';
              filter = 'all';
            }}>Clear filters</Button
          >{/if}
      </div>
    {:else}
      <ul class="object-list" id="sync-repository-list">
        {#each visible as row, index (row.repository)}
          {@const health = repositorySyncHealth(row)}
          {@const expanded = openRepository === row.repository}
          <li>
            <div class="object-row sync-repo-summary">
              <Button
                row
                class="row-hit"
                aria-expanded={expanded}
                aria-controls={`sync-repo-${index}`}
                onclick={() => {
                  openRepository = expanded ? null : row.repository;
                }}
              >
                <span class="visually-hidden">{row.repository} sync details</span>
              </Button>
              <span class="object-main">
                <span class="object-name file-path">{row.repository}</span>
              </span>
              <span class="object-side">
                <span class="repo-status cap-trim" data-health={health}>
                  <Icon
                    name={health === 'blocked'
                      ? 'alert'
                      : health === 'syncing'
                        ? 'refresh'
                        : health === 'paused'
                          ? 'minus-circle'
                          : 'check'}
                    size="sm"
                  />
                  <span class="band-trim">{healthWords[health]}</span>
                </span>
                <span class="repo-chevron" class:is-open={expanded}
                  ><Icon name="chevron-right" size="xs" /></span
                >
              </span>
            </div>
            {#if expanded}
              <div class="sync-repo-detail" id={`sync-repo-${index}`}>
                <dl>
                  {#each SYNC_KINDS as kind (kind)}
                    <div class="sync-kind-state">
                      <dt class="band-trim">{SYNC_SECTION_LABELS[kind]}</dt>
                      <dd class="band-trim">
                        {cellWords[row.cells[kind].state]}{row.cells[kind].changes
                          ? ` · ${row.cells[kind].changes} ${row.cells[kind].changes === 1 ? 'change' : 'changes'}`
                          : ''}
                      </dd>
                    </div>
                  {/each}
                </dl>
                {#if row.reason}<p class="object-sum">{row.reason}</p>{/if}
                <div class="object-side">
                  {#if plan?.actions.some((action) => action.repository === row.repository)}<Button
                      tone="quiet"
                      onclick={(event) => onDetails(event.currentTarget)}>View changes</Button
                    >{/if}
                  {#if repositoryHref}<Button tone="quiet" href={repositoryHref(row.repository)}
                      >Repository settings</Button
                    >{/if}
                </div>
              </div>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </Card>

  <Card labelledby="sync-configuration-heading" unsaved={dirtyControls.length > 0}>
    <div class="card-head">
      <h2 class="card-title" id="sync-configuration-heading">Shared configuration</h2>
      <span class="card-note band-trim">Changes take effect after saving</span>
    </div>
    <ul class="object-list">
      {#each SYNC_KINDS as kind (kind)}
        {@const config = configs[kind]}
        {@const dirty = dirtyControls.some((control) => control.startsWith(`sync.${kind}.`))}
        <li>
          <div class="object-row" class:is-unsaved={dirty} data-unsaved={dirty || undefined}>
            <a class="row-hit" href={sectionHref(kind)} onclick={(event) => open(event, kind)}>
              <span class="visually-hidden"
                >Open {SYNC_SECTION_LABELS[kind].toLowerCase()} configuration</span
              >
            </a>
            <span class="object-main">
              <span class="object-name sync-config-name cap-trim">
                <Icon name={kindIcons[kind]} size="sm" />
                <span class="band-trim">{SYNC_SECTION_LABELS[kind]}</span>
                <Icon name="chevron-right" size="xs" />
              </span>
              <span class="object-sum"
                >{kindSummary(kind)} · {dirty
                  ? config?.enabled
                    ? 'Will sync after saving'
                    : 'Will pause after saving'
                  : config?.enabled
                    ? kind === 'files'
                      ? 'Via pull requests'
                      : 'Applies automatically'
                    : 'Paused'}</span
              >
            </span>
            <span class="object-side sync-config-control">
              <Switch
                checked={config?.enabled === true}
                label={`${SYNC_SECTION_LABELS[kind]} sync`}
                bare
                disabled={readOnly || !config}
                onToggle={(next) => onToggleKind(kind, next)}
              />
            </span>
          </div>
        </li>
      {/each}
    </ul>
  </Card>
</section>

<style>
  /* Shared card, row, heading and copy classes own the panel's geometry.
     These rules describe only the sync content placed inside that anatomy. */
  .sync-home {
    min-width: 0;
  }
  h3.object-name {
    margin: 0;
  }
  .sync-totals {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: var(--space-4);
    margin-block: var(--space-6);
  }
  .sync-total {
    display: flex;
    flex-direction: column;
    gap: var(--row-copy-gap);
    color: var(--text-muted);
    font-size: var(--font-size-meta);
  }
  .sync-total strong {
    color: var(--text-primary);
    font-size: var(--font-size-page-title);
    line-height: var(--leading-page);
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }
  .sync-freshness {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    color: var(--text-muted);
    font-size: var(--font-size-meta);
  }
  .sync-progress {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-3);
    border-top: 1px solid var(--border-subtle);
    margin-top: var(--rhythm-card-head-body);
    padding-top: var(--rhythm-card-head-body);
  }
  .sync-progress .object-main {
    flex: 1 1 var(--setting-say-min);
  }
  .sync-issue-row {
    grid-template-columns: minmax(0, 1fr) auto;
  }
  .sync-filter-options {
    min-width: 0;
    max-width: 100%;
    overflow-x: auto;
    padding: 3px;
    margin: -3px;
  }
  .repo-status {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    white-space: nowrap;
  }
  .repo-status[data-health='blocked'] {
    color: var(--danger);
  }
  .repo-status[data-health='syncing'] {
    color: var(--info);
  }
  .repo-chevron {
    display: grid;
    place-items: center;
    color: var(--text-muted);
  }
  .repo-chevron.is-open {
    rotate: 90deg;
  }
  .sync-repo-detail {
    display: grid;
    gap: var(--rhythm-card-head-body);
    padding-block: var(--row-pad-default);
  }
  .sync-repo-detail dl {
    margin: 0;
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-4);
  }
  .sync-kind-state {
    display: grid;
    gap: var(--row-copy-gap);
  }
  .sync-kind-state dt {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
  }
  .sync-kind-state dd {
    margin: 0;
    font-size: var(--font-size-meta);
  }
  .sync-repo-detail p {
    margin: 0;
  }
  .sync-config-name {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
  }
  .sync-config-control :global(.switch) {
    /* The label owns the space around its track, independently of the row link.
       Paint the hit area without changing the shared row's text-driven rhythm. */
    padding: var(--space-3);
    margin: calc(-1 * var(--space-3));
  }
  .sync-empty {
    display: grid;
    justify-items: center;
    gap: var(--space-3);
  }
  @media (max-width: 36rem) {
    .sync-totals {
      gap: var(--space-2);
    }
    .sync-total {
      font-size: var(--font-size-micro);
    }
    .sync-issue-row {
      grid-template-columns: minmax(0, 1fr);
    }
    .sync-issue-row .object-side {
      justify-content: start;
    }
    .sync-repo-summary {
      gap: var(--space-2);
    }
    .sync-repo-summary .object-side {
      gap: var(--space-2);
    }
  }
</style>

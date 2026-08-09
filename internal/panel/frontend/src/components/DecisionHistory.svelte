<script lang="ts">
  import { formatDateTime, formatRelative, formatTimestamp } from '../lib/format';
  import type { AccessDecision } from '../lib/types';
  import Avatar from './Avatar.svelte';

  const {
    label,
    reason,
    decidedAt,
    fetchDecisions,
  }: {
    label: string;
    reason?: string;
    decidedAt?: string;
    fetchDecisions: () => Promise<AccessDecision[]>;
  } = $props();

  let details = $state<HTMLDetailsElement | null>(null);
  let trigger = $state<HTMLElement | null>(null);
  let decisions = $state<AccessDecision[] | null>(null);
  let loading = $state(false);
  let failure = $state<string | null>(null);
  const now = Date.now();

  $effect(() => {
    function outside(event: PointerEvent): void {
      if (
        details?.open === true &&
        event.target instanceof Node &&
        !details.contains(event.target)
      ) {
        close(false);
      }
    }
    function escape(event: KeyboardEvent): void {
      if (event.key !== 'Escape' || details?.open !== true) return;
      event.preventDefault();
      close(true);
    }
    document.addEventListener('pointerdown', outside);
    document.addEventListener('keydown', escape);
    return () => {
      document.removeEventListener('pointerdown', outside);
      document.removeEventListener('keydown', escape);
    };
  });

  function toggled(): void {
    if (details?.open === true && decisions === null && !loading) void load();
  }

  async function load(): Promise<void> {
    loading = true;
    failure = null;
    try {
      decisions = await fetchDecisions();
    } catch (error) {
      failure = error instanceof Error ? error.message : String(error);
    } finally {
      loading = false;
    }
  }

  function close(restoreFocus: boolean): void {
    if (details !== null) details.open = false;
    if (restoreFocus) trigger?.focus();
  }
</script>

<details class="decision-history" bind:this={details} ontoggle={toggled}>
  <summary bind:this={trigger} aria-label={label} title={label}>
    <span class="history-icon" aria-hidden="true"></span>
  </summary>
  <div class="decision-popover">
    <header>
      <div>
        <strong>{label}</strong>
        {#if decidedAt !== undefined}
          <time datetime={decidedAt} title={formatTimestamp(decidedAt)}>
            {formatDateTime(decidedAt)}
          </time>
        {/if}
      </div>
      <button type="button" aria-label="Close access history" onclick={() => close(true)}>×</button>
    </header>

    {#if reason !== undefined && reason.trim() !== ''}
      <div class="current-reason">
        <span class="mono">Reason</span>
        <p>{reason}</p>
      </div>
    {/if}

    <div class="decision-list" aria-live="polite">
      {#if loading}
        <p class="state dim">Loading decisions…</p>
      {:else if failure !== null}
        <p class="state form-error">{failure}</p>
      {:else}
        {#each decisions ?? [] as decision (decision.id)}
          <article>
            <Avatar account={decision.actor} size={24} />
            <div>
              <strong>{decision.summary}</strong>
              <span>
                {decision.actor.display_name} ·
                <time datetime={decision.created_at} title={formatTimestamp(decision.created_at)}>
                  {formatRelative(decision.created_at, now)}
                </time>
              </span>
              <code>{decision.action}</code>
            </div>
          </article>
        {:else}
          <p class="state dim">No earlier decisions in this scope</p>
        {/each}
      {/if}
    </div>
  </div>
</details>

<style>
  .decision-history {
    flex: none;
    position: relative;
  }

  .decision-history[open] {
    z-index: 34;
  }

  summary {
    align-items: center;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 50%;
    display: flex;
    height: 1.625rem;
    justify-content: center;
    width: 1.625rem;
  }

  summary::-webkit-details-marker {
    display: none;
  }

  summary::marker {
    content: '';
  }

  summary:hover,
  .decision-history[open] summary {
    background: var(--strip-lift);
    border-color: var(--control-border);
  }

  .history-icon {
    border: 1.5px solid var(--dim);
    border-radius: 50%;
    height: 0.8rem;
    position: relative;
    width: 0.8rem;
  }

  .history-icon::before,
  .history-icon::after {
    background: var(--dim);
    content: '';
    left: 0.34rem;
    position: absolute;
    top: 0.16rem;
    transform-origin: bottom;
  }

  .history-icon::before {
    height: 0.27rem;
    width: 1px;
  }

  .history-icon::after {
    height: 1px;
    top: 0.42rem;
    transform: rotate(28deg);
    width: 0.23rem;
  }

  .decision-popover {
    background: var(--strip);
    border: 1px solid var(--rule);
    border-radius: var(--r-ctl);
    box-shadow: 0 16px 40px var(--shadow);
    overflow: hidden;
    position: absolute;
    right: 0;
    top: calc(100% + 0.35rem);
    width: min(25rem, calc(100vw - 2rem));
    z-index: 34;
  }

  header {
    align-items: flex-start;
    border-bottom: 1px solid var(--rule);
    display: flex;
    justify-content: space-between;
    padding: 0.75rem;
  }

  header > div {
    display: flex;
    flex-direction: column;
  }

  header strong {
    font-size: 0.75rem;
  }

  header time {
    color: var(--dim);
    font: 0.625rem/1.4 var(--mono);
  }

  header button {
    background: transparent;
    border: 0;
    color: var(--dim);
    font-size: 1.1rem;
    line-height: 1;
  }

  .current-reason {
    background: var(--warning-tint);
    border-bottom: 1px solid color-mix(in srgb, var(--warning) 35%, transparent);
    padding: 0.625rem 0.75rem;
  }

  .current-reason span {
    color: var(--warning);
    font-size: 0.5625rem;
    text-transform: uppercase;
  }

  .current-reason p {
    font-size: 0.75rem;
    line-height: 1.4;
    margin: 0.2rem 0 0;
  }

  .decision-list {
    max-height: min(22rem, 52vh);
    overflow: auto;
    padding: 0.35rem;
  }

  article {
    display: grid;
    gap: 0.625rem;
    grid-template-columns: auto minmax(0, 1fr);
    padding: 0.55rem;
  }

  article + article {
    border-top: 1px solid var(--rule);
  }

  article > div {
    align-items: flex-start;
    display: flex;
    flex-direction: column;
  }

  article strong {
    font-size: 0.75rem;
    line-height: 1.35;
  }

  article span {
    color: var(--dim);
    font-size: 0.6875rem;
    line-height: 1.4;
  }

  article code {
    font-size: 0.5625rem;
    margin-top: 0.25rem;
  }

  .state {
    font-size: 0.75rem;
    margin: 0;
    padding: 0.75rem;
    text-align: center;
  }
</style>

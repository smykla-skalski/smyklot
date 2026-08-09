<script lang="ts">
  import { handleLabel, readHandle } from '../lib/identity';
  import type { PanelTarget, PanelViewer } from '../lib/types';
  import Avatar from './Avatar.svelte';

  const {
    viewer,
    iconUrl,
    targets,
    selectedId,
    targetHref,
    onSelectTarget,
    onSignOut,
  }: {
    viewer: PanelViewer | null;
    iconUrl: string;
    targets: PanelTarget[];
    selectedId: string | null;
    targetHref: (target: PanelTarget) => string;
    onSelectTarget: (targetId: string) => void;
    onSignOut: () => void | Promise<void>;
  } = $props();

  let accountMenu = $state<HTMLDetailsElement | null>(null);
  let accountTrigger = $state<HTMLElement | null>(null);

  const handle = $derived(
    viewer === null ? null : readHandle(viewer.account.provider, viewer.account.login),
  );
  const selectedTarget = $derived(
    selectedId === null ? null : (targets.find((target) => target.id === selectedId) ?? null),
  );
  const roleLabel = $derived(
    (selectedTarget?.effective_role ?? viewer?.global_role ?? 'none').toUpperCase(),
  );

  $effect(() => {
    function closeFromOutside(event: PointerEvent): void {
      if (
        accountMenu?.open === true &&
        event.target instanceof Node &&
        !accountMenu.contains(event.target)
      ) {
        accountMenu.open = false;
      }
    }

    document.addEventListener('pointerdown', closeFromOutside);
    document.addEventListener('keydown', closeFromKeyboard);
    return () => {
      document.removeEventListener('pointerdown', closeFromOutside);
      document.removeEventListener('keydown', closeFromKeyboard);
    };
  });

  function closeFromKeyboard(event: KeyboardEvent): void {
    if (event.key !== 'Escape' || accountMenu?.open !== true) return;
    event.preventDefault();
    accountMenu.open = false;
    accountTrigger?.focus();
  }

  async function signOut(): Promise<void> {
    if (accountMenu !== null) accountMenu.open = false;
    await onSignOut();
  }

  function selectTarget(event: MouseEvent, targetId: string): void {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey)
      return;
    event.preventDefault();
    if (accountMenu !== null) accountMenu.open = false;
    onSelectTarget(targetId);
  }
</script>

<header class="bar">
  <h1 class="mark">
    <img class="mark-icon" src={iconUrl} alt="" width="30" height="30" decoding="async" />
    <span class="mark-name">Smyklot</span>
    <span class="mark-part">Panel</span>
  </h1>

  {#if viewer !== null && handle !== null}
    <details class="account-menu" data-role={roleLabel} bind:this={accountMenu}>
      <summary class="who" bind:this={accountTrigger}>
        <span class="visually-hidden">{roleLabel}</span>
        <Avatar account={selectedTarget?.account ?? viewer.account} size={30} />
        <span class="who-text">
          <span class="who-name">{viewer.account.display_name}</span>
          <span class="who-context mono">
            {#if selectedTarget === null}
              {handleLabel(handle)}
            {:else}
              {selectedTarget.account.display_name}
            {/if}
          </span>
        </span>
        <span class="menu-chevron" aria-hidden="true"></span>
      </summary>
      <div class="account-popover">
        <p class="signed-in mono">Signed in as {handleLabel(handle)}</p>

        <div class="menu-section">
          <p class="menu-label mono">Installations</p>
          {#if targets.length === 0}
            <p class="empty-target dim">No installation is available</p>
          {:else}
            <div class="target-options">
              {#each targets as target (target.id)}
                <a
                  href={targetHref(target)}
                  class="target-option"
                  class:current={target.id === selectedId}
                  aria-current={target.id === selectedId ? 'true' : undefined}
                  onclick={(event) => selectTarget(event, target.id)}
                >
                  <Avatar account={target.account} size={26} />
                  <span class="option-copy">
                    <strong>{target.account.display_name}</strong>
                    <span class="mono">
                      @{target.account.login} · {target.type === 'Organization'
                        ? 'Organization'
                        : 'Personal'}
                    </span>
                  </span>
                  <span class="option-check" aria-hidden="true">
                    {target.id === selectedId ? '✓' : ''}
                  </span>
                </a>
              {/each}
            </div>
          {/if}
        </div>

        <div class="menu-separator" aria-hidden="true"></div>
        <button class="account-action" onclick={signOut}>Sign out</button>
      </div>
    </details>
  {/if}
</header>

<div class="brand-rule bar-rule" aria-hidden="true"></div>

<style>
  .bar {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: 0.75rem 1rem;
    justify-content: space-between;
    margin: 0 0 0.75rem;
    padding: 0 0.125rem;
  }

  .mark {
    align-items: center;
    display: flex;
    gap: 0.5rem;
    margin: 0;
  }

  .mark-icon {
    border-radius: 7px;
    flex: none;
  }

  .mark-name,
  .mark-part {
    font: 600 0.8125rem/1 var(--mono);
    letter-spacing: 0.2em;
    text-transform: uppercase;
  }

  .mark-part {
    color: var(--dim);
    font-weight: 500;
  }

  .bar-rule {
    margin-bottom: 1.25rem;
  }

  .account-menu {
    isolation: isolate;
    margin-left: 1rem;
    position: relative;
    z-index: 40;
  }

  .account-menu::before {
    background: var(--admin);
    border-radius: var(--r-ctl);
    bottom: 0;
    content: '';
    left: -1rem;
    pointer-events: none;
    position: absolute;
    right: 1rem;
    top: 0;
    transition: filter 120ms ease-out;
    z-index: 0;
  }

  .account-menu::after {
    color: var(--on-admin);
    content: attr(data-role);
    font: 900 0.5625rem/1 var(--sans);
    left: calc(-0.5rem + 1px);
    letter-spacing: 0.1em;
    pointer-events: none;
    position: absolute;
    top: 50%;
    transform: translate(-50%, -50%) rotate(-90deg);
    white-space: nowrap;
    z-index: 2;
  }

  .who {
    align-items: center;
    background: var(--well);
    border: 1px solid var(--admin);
    border-radius: var(--r-ctl);
    cursor: pointer;
    display: flex;
    gap: 0.625rem;
    min-height: 2.75rem;
    padding: 0.375rem 0.625rem;
    position: relative;
    user-select: none;
    z-index: 1;
  }

  .who::-webkit-details-marker {
    display: none;
  }

  .who::marker {
    content: '';
  }

  .who:hover,
  .account-menu[open] .who {
    background: var(--strip-lift);
  }

  .account-menu:has(.who:hover)::before,
  .account-menu[open]::before {
    filter: brightness(1.08);
  }

  .who-text {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    min-width: 0;
    text-align: left;
  }

  .who-name {
    font-size: 0.875rem;
    font-weight: 600;
    line-height: 1.2;
  }

  .who-context {
    color: var(--dim);
    font-size: 0.6875rem;
    line-height: 1.2;
    max-width: 15rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .menu-chevron {
    border-bottom: 1.5px solid var(--dim);
    border-right: 1.5px solid var(--dim);
    height: 0.45rem;
    margin: 0 0.2rem 0.2rem 0.25rem;
    transform: rotate(45deg);
    transition: transform 120ms ease-out;
    width: 0.45rem;
  }

  .account-menu[open] .menu-chevron {
    margin-bottom: -0.2rem;
    transform: rotate(225deg);
  }

  .account-popover {
    background: var(--strip);
    border: 1px solid var(--rule);
    border-radius: var(--r-ctl);
    box-shadow: 0 8px 24px var(--shadow);
    max-width: calc(100vw - 2rem);
    padding: 0.375rem;
    position: absolute;
    right: 0;
    top: calc(100% + 0.35rem);
    width: 21rem;
    z-index: 20;
  }

  .signed-in,
  .menu-label {
    color: var(--dim);
    font-size: 0.625rem;
    margin: 0;
  }

  .signed-in {
    padding: 0.4rem 0.5rem 0.5rem;
  }

  .menu-section {
    background: var(--well);
    border: 1px solid var(--rule);
    border-radius: calc(var(--r-ctl) - 2px);
    padding: 0.25rem;
  }

  .menu-label {
    letter-spacing: 0.1em;
    padding: 0.35rem 0.4rem 0.45rem;
    text-transform: uppercase;
  }

  .target-options {
    display: grid;
    gap: 0.15rem;
  }

  .target-option {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: calc(var(--r-ctl) - 4px);
    color: var(--text);
    display: grid;
    gap: 0.5rem;
    grid-template-columns: auto minmax(0, 1fr) 1rem;
    min-height: 2.75rem;
    padding: 0.35rem 0.4rem;
    text-align: left;
    text-decoration: none;
    width: 100%;
  }

  .target-option:hover,
  .target-option:focus-visible,
  .target-option.current {
    background: var(--strip-lift);
  }

  .option-copy {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .option-copy strong,
  .option-copy span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .option-copy strong {
    font-size: 0.75rem;
  }

  .option-copy span,
  .option-check,
  .empty-target {
    color: var(--dim);
    font-size: 0.625rem;
  }

  .option-check {
    color: var(--signal);
    font-weight: 700;
    text-align: center;
  }

  .empty-target {
    margin: 0;
    padding: 0.5rem;
  }

  .menu-separator {
    border-top: 1px solid var(--rule);
    margin: 0.375rem 0;
  }

  .account-action {
    background: transparent;
    border: 0;
    border-radius: calc(var(--r-ctl) - 2px);
    color: var(--text);
    display: block;
    font-size: 0.8125rem;
    height: var(--control-height);
    padding: 0 0.75rem;
    text-align: left;
    width: 100%;
  }

  .account-action:hover,
  .account-action:focus-visible {
    background: var(--strip-lift);
  }

  @media (max-width: 30rem) {
    .mark-part {
      display: none;
    }

    .who-context {
      max-width: 10rem;
    }
  }
</style>

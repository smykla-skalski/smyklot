<script lang="ts">
  import { PANEL_VIEWS, type PanelView } from '../lib/routes';

  const {
    value,
    hrefFor,
    onSelect,
    showUsers,
  }: {
    value: PanelView;
    hrefFor: (view: PanelView) => string;
    onSelect: (view: PanelView) => void;
    showUsers: boolean;
  } = $props();

  let settingsButton = $state<HTMLAnchorElement | null>(null);
  let repositoriesButton = $state<HTMLAnchorElement | null>(null);
  let historyButton = $state<HTMLAnchorElement | null>(null);
  let usersButton = $state<HTMLAnchorElement | null>(null);
  let helpButton = $state<HTMLAnchorElement | null>(null);
  const visibleViews = $derived(PANEL_VIEWS.filter((view) => view !== 'users' || showUsers));

  function selectFromClick(event: MouseEvent, next: PanelView): void {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey)
      return;
    event.preventDefault();
    onSelect(next);
  }

  function moveFromKeyboard(event: KeyboardEvent): void {
    let next: PanelView | null = null;
    switch (event.key) {
      case 'ArrowLeft': {
        const index = visibleViews.indexOf(value);
        next = visibleViews[(index - 1 + visibleViews.length) % visibleViews.length] ?? null;
        break;
      }
      case 'ArrowRight': {
        const index = visibleViews.indexOf(value);
        next = visibleViews[(index + 1) % visibleViews.length] ?? null;
        break;
      }
      case 'Home':
        next = 'settings';
        break;
      case 'End':
        next = 'help';
        break;
    }
    if (next === null) return;

    event.preventDefault();
    onSelect(next);
    switch (next) {
      case 'settings':
        settingsButton?.focus();
        break;
      case 'repositories':
        repositoriesButton?.focus();
        break;
      case 'history':
        historyButton?.focus();
        break;
      case 'users':
        usersButton?.focus();
        break;
      case 'help':
        helpButton?.focus();
        break;
    }
  }
</script>

<nav aria-label="Panel view">
  <div class="view-tabs" role="tablist" aria-orientation="horizontal">
    <a
      href={hrefFor('settings')}
      id="settings-tab"
      bind:this={settingsButton}
      class:active={value === 'settings'}
      role="tab"
      aria-selected={value === 'settings'}
      aria-controls="settings-panel"
      tabindex={value === 'settings' ? 0 : -1}
      onkeydown={moveFromKeyboard}
      onclick={(event) => selectFromClick(event, 'settings')}
    >
      <svg class="tab-icon" viewBox="0 0 20 20" aria-hidden="true">
        <path d="M3 5h5M12 5h5M3 10h9M16 10h1M3 15h2M9 15h8"></path>
        <circle cx="10" cy="5" r="1.5"></circle>
        <circle cx="14" cy="10" r="1.5"></circle>
        <circle cx="7" cy="15" r="1.5"></circle>
      </svg>
      Settings
    </a>
    <a
      href={hrefFor('repositories')}
      id="repositories-tab"
      bind:this={repositoriesButton}
      class:active={value === 'repositories'}
      role="tab"
      aria-selected={value === 'repositories'}
      aria-controls="repositories-panel"
      tabindex={value === 'repositories' ? 0 : -1}
      onkeydown={moveFromKeyboard}
      onclick={(event) => selectFromClick(event, 'repositories')}
    >
      <svg class="tab-icon" viewBox="0 0 20 20" aria-hidden="true">
        <path d="M5 3.5h10.5v13H6a2.5 2.5 0 0 1-2.5-2.5V5A1.5 1.5 0 0 1 5 3.5Z"></path>
        <path d="M6.5 3.5v13M6.5 13.5h9"></path>
      </svg>
      Repositories
    </a>
    {#if showUsers}
      <a
        href={hrefFor('users')}
        id="users-tab"
        bind:this={usersButton}
        class:active={value === 'users'}
        role="tab"
        aria-selected={value === 'users'}
        aria-controls="users-panel"
        tabindex={value === 'users' ? 0 : -1}
        onkeydown={moveFromKeyboard}
        onclick={(event) => selectFromClick(event, 'users')}
      >
        <svg class="tab-icon" viewBox="0 0 20 20" aria-hidden="true">
          <circle cx="7" cy="7" r="2.5"></circle>
          <circle cx="14" cy="8" r="2"></circle>
          <path d="M2.5 16c.3-3 2-4.5 4.5-4.5s4.2 1.5 4.5 4.5M11 12.5c2.8-.8 5.2.5 5.8 3.5"></path>
        </svg>
        Users
      </a>
    {/if}
    <a
      href={hrefFor('history')}
      id="history-tab"
      bind:this={historyButton}
      class:active={value === 'history'}
      role="tab"
      aria-selected={value === 'history'}
      aria-controls="history-panel"
      tabindex={value === 'history' ? 0 : -1}
      onkeydown={moveFromKeyboard}
      onclick={(event) => selectFromClick(event, 'history')}
    >
      <svg class="tab-icon" viewBox="0 0 20 20" aria-hidden="true">
        <circle cx="10" cy="10" r="7"></circle>
        <path d="M10 6v4l3 2"></path>
      </svg>
      History
    </a>
    <a
      href={hrefFor('help')}
      id="help-tab"
      bind:this={helpButton}
      class="help-tab"
      class:active={value === 'help'}
      role="tab"
      aria-selected={value === 'help'}
      aria-controls="help-panel"
      tabindex={value === 'help' ? 0 : -1}
      onkeydown={moveFromKeyboard}
      onclick={(event) => selectFromClick(event, 'help')}
    >
      <svg class="tab-icon" viewBox="0 0 20 20" aria-hidden="true">
        <circle cx="10" cy="10" r="7"></circle>
        <circle cx="10" cy="10" r="2.5"></circle>
        <path d="m5 5 3.2 3.2M11.8 11.8 15 15M15 5l-3.2 3.2M8.2 11.8 5 15"></path>
      </svg>
      Help
    </a>
  </div>
</nav>

<style>
  .view-tabs {
    border-bottom: 1px solid var(--rule);
    display: flex;
    gap: 0.25rem;
    margin: 0 0 1rem;
    overflow-x: auto;
    padding: 0 0.5rem;
    scrollbar-width: thin;
  }

  a {
    align-items: center;
    background: transparent;
    border: 0;
    border-bottom: 2px solid transparent;
    color: var(--dim);
    display: inline-flex;
    font-size: 0.875rem;
    font-weight: 600;
    gap: 0.4rem;
    height: 2.75rem;
    margin-bottom: -1px;
    padding: 0 1rem;
    text-decoration: none;
    transition:
      border-color 140ms ease-out,
      color 140ms ease-out;
  }

  a:hover {
    color: var(--text);
  }

  a.active {
    border-bottom-color: var(--accent);
    color: var(--text);
  }

  .tab-icon {
    fill: none;
    flex: none;
    height: 1rem;
    stroke: currentColor;
    stroke-linecap: round;
    stroke-linejoin: round;
    stroke-width: 1.4;
    width: 1rem;
  }

  a.active .tab-icon {
    color: var(--accent);
  }

  .help-tab {
    margin-left: auto;
  }
</style>

<script lang="ts">
  import { Combobox } from 'bits-ui';
  import { untrack } from 'svelte';
  import { useDebounce } from 'runed';

  import type { PanelAccount } from '../types';
  import Avatar from './Avatar.svelte';

  let {
    id,
    value = $bindable(''),
    label,
    help,
    refused = false,
    suggest,
    focusOnOpen = false,
  }: {
    id: string;
    value: string;
    label: string;
    help?: string;
    refused?: boolean;
    suggest: (query: string) => Promise<PanelAccount[]>;
    focusOnOpen?: boolean;
  } = $props();

  let items = $state.raw<PanelAccount[]>([]);
  let open = $state(false);
  let selectedValue = $state('');
  let field = $state<HTMLInputElement | null>(null);
  let generation = 0;
  let accepted = $state<string | null>(null);
  const choices = $derived(
    items.map((account) => ({
      value: account.login,
      label: account.display_name || account.login,
    })),
  );

  const debouncedSuggest = useDebounce((query: string) => void load(query), 180);
  $effect(() => {
    const query = value.trim();
    untrack(() => {
      if (query.length < 2 || query === accepted) {
        items = [];
        open = false;
        return;
      }
      void debouncedSuggest(query);
    });
  });

  async function load(query: string): Promise<void> {
    const mine = ++generation;
    const found = await suggest(query);
    if (mine !== generation || value.trim() !== query) return;
    items = found;
    open = found.length > 0;
  }

  function typed(event: Event): void {
    const input = event.currentTarget as HTMLInputElement;
    accepted = null;
    selectedValue = '';
    value = input.value;
  }

  function choose(login: string): void {
    const account = items.find((item) => item.login === login);
    if (account === undefined) return;
    accepted = account.login;
    selectedValue = account.login;
    value = account.login;
    items = [];
    open = false;
    queueMicrotask(() => field?.focus());
  }
</script>

<label class="form-field login-field" for={id}>
  <span>{label}</span>
  <Combobox.Root
    type="single"
    items={choices}
    bind:open
    bind:value={selectedValue}
    inputValue={value}
    onValueChange={choose}
  >
    <Combobox.Input
      {id}
      bind:ref={field}
      class="text-input login-input"
      autocomplete="off"
      spellcheck="false"
      autocapitalize="none"
      placeholder="octocat"
      oninput={typed}
      required
      data-modal-focus={focusOnOpen ? true : undefined}
    />
    <Combobox.Portal to=".app-shell">
      <Combobox.Content class="suggestion-content" sideOffset={4} collisionPadding={8}>
        <Combobox.Viewport class="suggestions" aria-label={label}>
          {#each items as account (account.id)}
            <Combobox.Item class="suggestion-item" value={account.login} label={account.login}>
              <Avatar {account} size={20} />
              <span class="suggestion-login">{account.login}</span>
              {#if account.display_name !== '' && account.display_name !== account.login}
                <span class="suggestion-name">{account.display_name}</span>
              {/if}
            </Combobox.Item>
          {/each}
        </Combobox.Viewport>
      </Combobox.Content>
    </Combobox.Portal>
  </Combobox.Root>
  {#if help !== undefined}
    <small class="identity-help" class:refused>{help}</small>
  {/if}
</label>

<style>
  .form-field {
    align-content: start;
    display: grid;
    gap: 0.4rem;
  }

  .form-field > span {
    font: 600 0.75rem / 1 var(--sans);
  }

  :global(.login-input) {
    width: 100%;
  }

  .identity-help {
    color: var(--dim);
    font-size: 0.6875rem;
    font-weight: 400;
    line-height: 1.35;
    margin-top: -0.05rem;
  }

  .identity-help.refused {
    color: var(--stop);
    font-weight: 500;
  }

  :global(.suggestion-content) {
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: var(--radius-popover);
    box-shadow: var(--shadow-popover);
    max-height: var(--bits-floating-available-height);
    min-width: var(--bits-floating-anchor-width);
    overflow: auto;
    z-index: var(--layer-dialog-popover);
  }

  :global(.suggestions) {
    display: grid;
    gap: 2px;
    padding: var(--space-1);
  }

  :global(.suggestion-item) {
    align-items: center;
    border-radius: var(--radius-control);
    color: var(--text-secondary);
    cursor: pointer;
    display: grid;
    gap: var(--space-2);
    grid-template-columns: 1.25rem auto minmax(0, 1fr);
    min-height: var(--control-height-compact);
    outline: 0;
    padding: 0 var(--space-2);
  }

  :global(.suggestion-item[data-highlighted]) {
    background: var(--interactive-hover);
    color: var(--text-primary);
  }

  :global(.suggestion-login) {
    font: 600 var(--font-size-compact) / 1 var(--sans);
  }

  :global(.suggestion-name) {
    color: var(--text-secondary);
    font: var(--font-size-compact) / 1 var(--sans);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>

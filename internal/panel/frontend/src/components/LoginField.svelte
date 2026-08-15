<script lang="ts">
  import { untrack } from 'svelte';

  import type { PanelAccount } from '../lib/types';
  import Avatar from './Avatar.svelte';
  import Popover from './Popover.svelte';

  /**
   * A GitHub login, typed, with the people who could be meant offered underneath.
   *
   * It stays a text field. Whatever is typed is what gets submitted, and the
   * suggestions are an offer: an installation on a personal account has no roster
   * to draw from, somebody outside the organization will never appear in one, and
   * the panel resolves the login on submit either way. So there is no "not found"
   * state here and no way for this to block a name it has not heard of.
   *
   * The pattern is the ARIA combobox with `aria-autocomplete="list"`: the field
   * owns the value, the listbox only proposes. Nothing is written into the field
   * by moving through the list, because that would overwrite what somebody is
   * still typing - the highlighted row is announced instead, and Enter takes it.
   */
  // `let` rather than `const`: choosing a suggestion writes the bound value back.
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
    /** Marks the help line as a refusal rather than guidance. */
    refused?: boolean;
    /** Reads candidates for what has been typed; returns none when unavailable. */
    suggest: (query: string) => Promise<PanelAccount[]>;
    focusOnOpen?: boolean;
  } = $props();

  /** Long enough that a keystroke inside a word does not cost a request. */
  const SETTLE_MS = 180;

  let items = $state<PanelAccount[]>([]);
  let highlighted = $state(-1);
  let open = $state(false);
  let field = $state<HTMLInputElement>();
  /** Rises with each request so a slow answer cannot overwrite a newer one. */
  let generation = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;
  /** The login taken from the list, so taking it does not re-ask for itself. */
  let accepted = $state<string | null>(null);

  const listId = $derived(`${id}-suggestions`);
  const activeId = $derived(highlighted === -1 ? undefined : `${listId}-${highlighted}`);

  /* Typing reopens the list; choosing or dismissing closes it until the next
     keystroke. Watching the value rather than the keystroke means a value put
     there by anything else - a reissued invitation filling the login in - does
     not silently drop a stale list underneath it. */
  $effect(() => {
    const query = value.trim();
    untrack(() => schedule(query));
  });

  $effect(() => () => clearTimeout(timer));

  function schedule(query: string): void {
    clearTimeout(timer);
    /* A login just taken from the list is an answer, not a question. Without
       this, choosing one asks for completions of itself and the list re-opens
       holding the single name that was picked. */
    if (query.length < 2 || query === accepted) {
      items = [];
      highlighted = -1;
      return;
    }
    timer = setTimeout(() => void load(query), SETTLE_MS);
  }

  async function load(query: string): Promise<void> {
    generation += 1;
    const mine = generation;
    const found = await suggest(query);
    if (mine !== generation) return;
    items = found;
    highlighted = -1;
    /* Only ever opened by an answer, so the list cannot flash empty while the
       first request is still out. */
    open = found.length > 0;
  }

  function choose(account: PanelAccount): void {
    accepted = account.login;
    value = account.login;
    open = false;
    items = [];
    highlighted = -1;
    field?.focus();
  }

  function keydown(event: KeyboardEvent): void {
    if (event.key === 'Escape' && open) {
      /* Swallowed here rather than left to close the dialog around the field:
         the first Escape dismisses the list, a second one leaves. */
      event.preventDefault();
      event.stopPropagation();
      open = false;
      highlighted = -1;
      return;
    }
    if (items.length === 0) return;

    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      open = true;
      const step = event.key === 'ArrowDown' ? 1 : -1;
      const count = items.length;
      highlighted =
        highlighted === -1 ? (step === 1 ? 0 : count - 1) : (highlighted + step + count) % count;
      return;
    }
    if (event.key === 'Enter' && open && highlighted !== -1) {
      const picked = items[highlighted];
      if (picked === undefined) return;
      // Enter on a highlighted row takes it; it must not also submit the form.
      event.preventDefault();
      choose(picked);
    }
  }
</script>

<!-- Two roots, no wrapper: the label is the grid item its parent lays out, and a
     wrapper around it would be the item instead and take the whole row. The list
     needs no positioned ancestor - it is in the top layer, placed by measurement. -->
<label class="form-field login-field" for={id}>
  <span>{label}</span>
  <input
    {id}
    bind:this={field}
    class="text-input"
    autocomplete="off"
    spellcheck="false"
    autocapitalize="none"
    placeholder="octocat"
    role="combobox"
    aria-expanded={open}
    aria-controls={listId}
    aria-autocomplete="list"
    aria-activedescendant={activeId}
    bind:value
    onkeydown={keydown}
    onblur={() => (open = false)}
    required
    data-modal-focus={focusOnOpen ? true : undefined}
  />
  {#if help !== undefined}
    <small class="identity-help" class:refused>{help}</small>
  {/if}
</label>

<!--
  The list is in the top layer, not in the form: these fields stand inside a
  dialog whose body scrolls, and a scroll container clips whatever is positioned
  inside it - the list was cut off at the body's edge and what showed through
  there was the dialog's footer.

  Manual rather than the platform's light dismiss, which fires on any pointerdown
  including one in the field being typed into. This one is opened by an answer
  arriving and closed by choosing, Escape, or leaving the field - and it never
  takes focus, because the field has it and is still being typed in.
-->
<Popover
  bind:open
  anchor={field ?? null}
  dismiss="manual"
  width="trigger"
  offset={4}
  focusOnOpen={false}
>
  <ul class="suggestions" id={listId} role="listbox" aria-label={label}>
    {#each items as account, index (account.id)}
      <li
        id={`${listId}-${index}`}
        role="option"
        aria-selected={index === highlighted}
        class:highlighted={index === highlighted}
      >
        <!-- Pressed on pointerdown, before the field's blur closes the list. -->
        <button
          type="button"
          tabindex="-1"
          onpointerdown={(event) => {
            event.preventDefault();
            choose(account);
          }}
          onmouseenter={() => (highlighted = index)}
        >
          <Avatar {account} size={20} />
          <span class="suggestion-login">{account.login}</span>
          {#if account.display_name !== '' && account.display_name !== account.login}
            <span class="suggestion-name">{account.display_name}</span>
          {/if}
        </button>
      </li>
    {/each}
  </ul>
</Popover>

<style>
  /* The field owns its own stack rather than inheriting one. A parent's scoped
     rules do not reach a child component's root element, so the `.form-field`
     styling next to it in the dialog would leave the label sitting inline with
     the input here. Same shape, declared where it applies. */
  .form-field {
    /* `start`, not the default `normal`: these fields sit in a row stretched by
       whichever column carries help text, and a stretched auto row shares the
       slack out, pushing the control beside this one out of line with it. */
    align-content: start;
    display: grid;
    gap: 0.4rem;
  }

  .form-field > span {
    font: 600 0.75rem / 1 var(--sans);
  }

  .form-field .text-input {
    width: 100%;
  }

  .identity-help {
    color: var(--dim);
    font-size: 0.6875rem;
    font-weight: 400;
    line-height: 1.35;
    /* The grid gap is 0.4rem; the mock puts 0.35rem above helpers. */
    margin-top: -0.05rem;
  }

  .identity-help.refused {
    color: var(--stop);
    font-weight: 500;
  }

  /* The list stands over what follows it in the dialog rather than pushing the
     rest of the form down each time a letter is typed. The surface it stands on
     belongs to the layer around it; this is only the rows. */
  .suggestions {
    /* Two pixels, so a highlighted row and the one below it never meet along an
       edge and read as one taller block. */
    display: grid;
    gap: 2px;
    list-style: none;
    margin: 0;
    padding: var(--space-1);
  }

  .suggestions button {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: var(--radius-control);
    color: var(--text-secondary);
    cursor: pointer;
    display: grid;
    gap: var(--space-2);
    grid-template-columns: 1.25rem auto minmax(0, 1fr);
    min-height: var(--control-height-compact);
    padding: 0 var(--space-2);
    text-align: left;
    width: 100%;
  }

  .suggestions li.highlighted button {
    background: var(--interactive-hover);
    color: var(--text-primary);
  }

  .suggestions button:active {
    scale: var(--press-scale);
  }

  .suggestion-login {
    font: 600 var(--font-size-compact) / 1 var(--sans);
    text-box: trim-both cap alphabetic;
  }

  .suggestion-name {
    color: var(--text-secondary);
    font: var(--font-size-compact) / 1 var(--sans);
    overflow: hidden;
    text-box: trim-both cap alphabetic;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>

<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { useDebounce } from 'runed';

  import { findMarks, findMatches, SEARCH_MINIMUM } from '#lib/components/FindPalette.svelte';
  import type { FindEntry } from '#lib/components/FindPalette.svelte';
  import Button from '#lib/components/Button.svelte';
  import Card from '#lib/components/Card.svelte';
  import Icon from '#lib/components/Icon.svelte';
  import PageHeader from '#lib/components/PageHeader.svelte';
  import Pill from '#lib/components/Pill.svelte';
  import SearchField from '#lib/components/SearchField.svelte';
  import { getFinder } from '#lib/finder.svelte.js';
  import { searchAddress } from '#lib/addresses.js';

  const finder = getFinder();

  /* The address is the query. A reader who searched, opened a result and pressed back
     is owed the search they left, and a page whose state lives only in a variable
     cannot give it to them. */
  const asked = $derived(page.url.searchParams.get('q') ?? '');
  /* The field follows the address until the reader types, and then leads it: writable
     because typing is the one thing allowed to get ahead of the query string. */
  let typed = $derived(asked);
  let looked = $state<FindEntry[]>([]);
  let looking = $state(false);
  let ranFor = $state<string | null>(null);

  /* The same floor the palette asks at, from the same constant: one letter matches most
     of the panel, and the two of them are one search. */
  const asking = $derived(asked.trim().length >= SEARCH_MINIMUM);
  const terms = $derived(
    !asking
      ? []
      : asked
          .trim()
          .toLowerCase()
          .split(/\s+/u)
          .filter((term) => term !== ''),
  );

  /* Everything, from both consoles: the palette holds its results to the console the
     reader is in until they ask for more, because it is a list of twelve rows over the
     page they are reading. A page whose whole job is the results has no reason to. */
  const hits = $derived(
    terms.length === 0
      ? []
      : [...finder().entries, ...looked].filter((entry) => findMatches(entry, terms)),
  );

  const groups = $derived.by(() => {
    const order: { name: string; cross?: string; rows: FindEntry[] }[] = [];
    for (const entry of hits) {
      const key = `${entry.cross ?? ''}:${entry.group}`;
      const held = order.find((group) => `${group.cross ?? ''}:${group.name}` === key);
      if (held === undefined) order.push({ name: entry.group, cross: entry.cross, rows: [entry] });
      else held.rows.push(entry);
    }
    return order;
  });

  const search = useDebounce((next: string) => {
    void goto(searchAddress(next), { replace: true, reset: false });
  }, 250);

  /* Repositories and people are asked for, and only once per query: the lookup is the
     one thing on this page that costs a request. */
  $effect(() => {
    const query = asked.trim();
    const lookup = finder().lookup;
    if (!asking || lookup === undefined) {
      looked = [];
      ranFor = query;
      return;
    }
    if (ranFor === query) return;
    ranFor = query;
    looking = true;
    void lookup(query)
      .then((found) => {
        if (ranFor === query) looked = found;
      })
      .catch(() => {
        if (ranFor === query) looked = [];
      })
      .finally(() => {
        if (ranFor === query) looking = false;
      });
  });

  function clear(): void {
    typed = '';
    void goto(searchAddress(''), { replace: true, reset: false });
  }
</script>

<!--
@component
Everything the finder can reach for one query, on a page of its own.

The palette is a list of twelve rows over the page a reader is reading, and it holds
itself to the console they are in. This is where the rest of it lives: both consoles,
every match, grouped the way the palette groups them so the two read as one search
rather than as two that happen to share a field.
-->

<div class="view-frame">
  <PageHeader
    id="search-heading"
    eyebrow={finder().scopeName}
    title="Search results"
    description="Pages, workspaces, repositories and people - from here and from {finder()
      .crossLabel}"
  />

  <div class="filter-bar">
    <SearchField
      value={typed}
      placeholder="Search"
      label="Search"
      onInput={(next) => {
        typed = next;
        void search(next);
      }}
    />
  </div>

  <Card>
    {#if terms.length === 0}
      <div class="state-panel">
        {#if asked.trim() === ''}
          <span
            ><strong>Enter a search term.</strong> Find pages, repositories, settings, queue work and
            people</span
          >
        {:else}
          <span
            ><strong>Keep typing.</strong> A search starts at {SEARCH_MINIMUM} letters - one matches most
            of the panel</span
          >
        {/if}
      </div>
    {:else if hits.length === 0 && !looking}
      <div class="state-panel">
        <span><strong>Nothing matches “{asked}”.</strong> Check the spelling, or start again</span>
        <Button onclick={clear}>Clear the search</Button>
      </div>
    {:else}
      {#each groups as group (`${group.cross ?? ''}:${group.name}`)}
        <div class="card-head">
          <h2 class="card-title">{group.name}</h2>
          {#if group.cross !== undefined}
            <span class="card-meta">in {group.cross}</span>
          {/if}
        </div>
        <ul class="object-list">
          <!-- Keyed on what the row SAYS, not on where it goes: several people all lead
               to the same access page, and an href key made two of them one row. -->
          {#each group.rows as entry (`${entry.title}:${entry.say}`)}
            <li>
              <a
                class="object-row"
                href={entry.href}
                onclick={(event) => {
                  if (event.metaKey || event.ctrlKey || event.shiftKey || event.button !== 0)
                    return;
                  event.preventDefault();
                  entry.select();
                }}
              >
                <span class="object-main">
                  <span class="object-name-row">
                    <span class="object-name">
                      {#each findMarks(entry.title, terms) as part, index (index)}{#if part.hit}<mark
                            >{part.text}</mark
                          >{:else}{part.text}{/if}{/each}
                    </span>
                    {#if entry.cross !== undefined}
                      <Pill>{entry.cross}</Pill>
                    {/if}
                  </span>
                  <span class="object-sum">
                    {#each findMarks(entry.say, terms) as part, index (index)}{#if part.hit}<mark
                          >{part.text}</mark
                        >{:else}{part.text}{/if}{/each}
                  </span>
                </span>
                <span class="object-side" aria-hidden="true"
                  ><Icon name="chevron-right" size="xs" /></span
                >
              </a>
            </li>
          {/each}
        </ul>
      {/each}

      {#if hits.length > 0}
        <div class="list-foot">
          <span
            >{hits.length}
            {hits.length === 1 ? 'result' : 'results'} for “{asked}”</span
          >
        </div>
      {/if}
    {/if}
  </Card>
</div>

<style>
  /* A group head after the first opens against the list above it rather than against the
     card's own frame. */
  .card-head:not(:first-child) {
    margin-block-start: var(--rhythm-card-gap);
  }

  /* The matched run is said in the finder's own ink, not on a browser's yellow ground -
     the same mark the palette makes, because they are one search. */
  mark {
    background: none;
    color: var(--match-ink);
    font-weight: 650;
  }
</style>

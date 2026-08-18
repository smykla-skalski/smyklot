<script lang="ts">
  import { onMount } from 'svelte';

  /**
   * A table of design tokens, showing what the panel is actually painting.
   *
   * Values are read from the live `.app-shell` element rather than restated from
   * `MASTER.md`, for two reasons. The obvious one is that a copied hex code goes
   * stale the day someone edits the palette. The other is that `.app-shell.root-mode`
   * **re-declares** the whole alias set - custom properties resolve at computed-value
   * time and cannot be inherited into it - so reading `:root` would report the panel's
   * values while the Root console is on screen, which is exactly the mistake these
   * pages exist to prevent.
   */
  const {
    tokens,
    swatch = true,
  }: {
    tokens: ReadonlyArray<{ name: string; note?: string }>;
    /** Draw the value as a colour. Off for sizes, radii and durations. */
    swatch?: boolean;
  } = $props();

  let host = $state<HTMLElement | null>(null);
  let values = $state<Record<string, string>>({});

  function read(): void {
    // The shell, not the root: `.root-mode` re-declares the aliases on it.
    const shell = host?.closest('.app-shell') ?? document.documentElement;
    const style = getComputedStyle(shell);
    values = Object.fromEntries(
      tokens.map(({ name }) => [name, style.getPropertyValue(`--${name}`).trim()]),
    );
  }

  onMount(() => {
    read();
    /* The toolbar changes `data-theme` on the document element and `.root-mode` on
       the shell, and neither is a Svelte state this page can depend on - so watch the
       attributes themselves. */
    const observer = new MutationObserver(() => read());
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['data-theme'],
    });
    const shell = host?.closest('.app-shell');
    if (shell !== null && shell !== undefined) {
      observer.observe(shell, { attributes: true, attributeFilter: ['class'] });
    }
    return () => observer.disconnect();
  });
</script>

<div class="token-table" bind:this={host}>
  <table>
    <thead>
      <tr>
        {#if swatch}<th scope="col" class="swatch-column"><span class="cap-trim">Value</span></th
          >{/if}
        <th scope="col"><span class="cap-trim">Token</span></th>
        <th scope="col"><span class="cap-trim">Resolves to</span></th>
        <th scope="col"><span class="cap-trim">What it is for</span></th>
      </tr>
    </thead>
    <tbody>
      {#each tokens as token (token.name)}
        <tr>
          {#if swatch}
            <td class="swatch-cell">
              <span class="swatch" style:background={`var(--${token.name})`}></span>
            </td>
          {/if}
          <th scope="row"><code class="mono">--{token.name}</code></th>
          <td><code class="mono value">{values[token.name] || '—'}</code></td>
          <td class="note">{token.note ?? ''}</td>
        </tr>
      {/each}
    </tbody>
  </table>
</div>

<style>
  .token-table {
    overflow-x: auto;
  }

  table {
    border-collapse: collapse;
    width: 100%;
  }

  th,
  td {
    border-bottom: 1px solid var(--rule);
    padding: var(--space-2) var(--space-3);
    text-align: left;
    vertical-align: middle;
  }

  thead th {
    color: var(--text-muted);
    font: 700 var(--font-size-micro) / 1 var(--sans);
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  tbody th {
    font-weight: 400;
  }

  .swatch-column {
    width: 4rem;
  }

  .swatch-cell {
    width: 4rem;
  }

  .swatch {
    border: 1px solid var(--control-border);
    border-radius: var(--radius-control);
    display: block;
    height: 1.75rem;
    width: 3rem;
  }

  .value {
    color: var(--text-secondary);
  }

  .note {
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
    max-width: 28rem;
  }

  code {
    font-size: var(--font-size-compact);
  }
</style>

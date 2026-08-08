<script lang="ts">
  import { HELP_COMMANDS, filterHelpCommands } from '../lib/help';

  let query = $state('');
  let copyState = $state<string | null>(null);
  let resetCopyState: ReturnType<typeof setTimeout> | undefined;

  const commands = $derived(filterHelpCommands(query));

  async function copy(text: string, key: string): Promise<void> {
    if (resetCopyState !== undefined) clearTimeout(resetCopyState);
    try {
      await navigator.clipboard.writeText(text);
      copyState = key;
    } catch {
      copyState = 'failed';
    }
    resetCopyState = setTimeout(() => {
      copyState = null;
      resetCopyState = undefined;
    }, 1600);
  }

  $effect(() => () => {
    if (resetCopyState !== undefined) clearTimeout(resetCopyState);
  });
</script>

<article class="help-page" aria-labelledby="help-title">
  <div class="help-hero-shell">
    <header class="help-hero">
      <div class="hero-copy">
        <p class="eyebrow">Smyklot guide</p>
        <h2 id="help-title">Review and merge with confidence</h2>
        <p>
          Use a comment or reaction. Smyklot verifies ownership, repository settings, and required
          checks before acting
        </p>
      </div>
      <div class="hero-example" aria-label="Recommended first command">
        <code>/approve</code>
        <button class="copy-button" type="button" onclick={() => copy('/approve', 'hero')}>
          {copyState === 'hero' ? 'Copied' : 'Copy'}
        </button>
      </div>
    </header>
  </div>

  <nav class="topic-nav" aria-label="Help topics">
    <a href="#help-start">Quick start</a>
    <a href="#help-commands">Commands</a>
    <a href="#help-configuration">Configuration</a>
    <a href="#help-troubleshooting">Troubleshooting</a>
  </nav>

  <section class="help-section" id="help-start" aria-labelledby="help-start-title">
    <div class="section-heading">
      <div>
        <p class="eyebrow">Quick start</p>
        <h3 id="help-start-title">Approve your first pull request</h3>
      </div>
    </div>

    <div class="quick-start-layout">
      <div class="quick-panel">
        <div class="quick-panel-heading">
          <h4>Prepare</h4>
          <span class="mono">3 STEPS</span>
        </div>
        <ol class="steps">
          <li>
            <span class="step-number mono">01</span>
            <div>
              <strong>Enable a repository</strong>
              <p>Set it to On in Repositories or inherit an On account setting</p>
            </div>
          </li>
          <li>
            <span class="step-number mono">02</span>
            <div>
              <strong>Confirm ownership</strong>
              <p>
                Add your handle to the global <code>*</code> rule in
                <code>.github/CODEOWNERS</code>
              </p>
            </div>
          </li>
          <li>
            <span class="step-number mono">03</span>
            <div>
              <strong>Open a pull request</strong>
              <p>Smyklot handles commands on its conversation</p>
            </div>
          </li>
        </ol>
      </div>

      <div class="quick-panel">
        <div class="quick-panel-heading">
          <h4>Invoke</h4>
          <span class="mono">CHOOSE ONE</span>
        </div>
        <div class="invocation-grid">
          <div class="invocation-card recommended">
            <div class="method-copy">
              <strong>Slash command</strong>
              <p>Recommended for pull request comments</p>
            </div>
            <code>/approve</code>
          </div>
          <div class="invocation-card">
            <div class="method-copy">
              <strong>Mention</strong>
              <p>When mention commands are enabled</p>
            </div>
            <code>@smyklot approve</code>
          </div>
          <div class="invocation-card">
            <div class="method-copy">
              <strong>Bare command</strong>
              <p>Exact command or alias only</p>
            </div>
            <code>lgtm</code>
          </div>
        </div>
      </div>
    </div>
  </section>

  <section class="help-section" id="help-commands" aria-labelledby="help-commands-title">
    <div class="section-heading command-heading">
      <div>
        <p class="eyebrow">Command reference</p>
        <h3 id="help-commands-title">Ask for one action or build a sequence</h3>
      </div>
      <label class="command-search">
        <span class="visually-hidden">Search commands</span>
        <svg viewBox="0 0 20 20" aria-hidden="true">
          <circle cx="8.5" cy="8.5" r="5.5"></circle>
          <path d="m12.5 12.5 4 4"></path>
        </svg>
        <input bind:value={query} type="search" placeholder="Search commands" />
      </label>
    </div>

    <p class="visually-hidden" aria-live="polite">
      {commands.length} OF {HELP_COMMANDS.length} COMMANDS
    </p>

    {#if commands.length === 0}
      <div class="empty-search">
        <strong>No command matches “{query.trim()}”</strong>
        <button type="button" onclick={() => (query = '')}>Show all commands</button>
      </div>
    {:else}
      <div class="command-table-wrap">
        <table class="command-table">
          <colgroup>
            <col class="command-column" />
            <col />
            <col class="example-column" />
          </colgroup>
          <thead>
            <tr>
              <th scope="col">Command</th>
              <th scope="col">Action</th>
              <th scope="col">Example</th>
            </tr>
          </thead>
          <tbody>
            {#each commands as command (command.name)}
              <tr>
                <th scope="row">
                  <div class="command-identity">
                    <span class="command-name">{command.name}</span>
                    {#if command.aliases.length > 0}
                      <span class="aliases mono">{command.aliases.join(' · ')}</span>
                    {/if}
                  </div>
                </th>
                <td>{command.summary}</td>
                <td class="command-example-cell">
                  <button
                    class="command-example mono"
                    type="button"
                    aria-label={`Copy ${command.example}`}
                    onclick={() => copy(command.example, command.name)}
                  >
                    <code>{command.example}</code>
                    <span
                      class="command-copy-overlay"
                      class:visible={copyState === command.name}
                      aria-hidden="true"
                    >
                      {#if copyState === command.name}
                        <svg viewBox="0 0 16 16">
                          <path d="m3.5 8.25 2.75 2.75 6.25-6.25"></path>
                        </svg>
                        Copied
                      {:else}
                        <svg viewBox="0 0 16 16">
                          <rect x="5.25" y="4.5" width="7" height="8" rx="1"></rect>
                          <path d="M10.25 4.5v-1h-7v8h2"></path>
                        </svg>
                        Copy
                      {/if}
                    </span>
                  </button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}

    <div class="ci-callout">
      <div class="ci-icon" aria-hidden="true">
        <svg viewBox="0 0 20 20">
          <circle cx="10" cy="10" r="7"></circle>
          <path d="M10 6v4l3 2"></path>
        </svg>
      </div>
      <div>
        <h4>Wait for checks without returning later</h4>
        <p>
          Add <code>after CI</code> or <code>after required checks</code> to a merge command. Smyklot
          waits, then merges when the selected checks pass
        </p>
      </div>
      <button
        class="copy-button callout-copy"
        type="button"
        onclick={() => copy('/approve /merge after required checks', 'ci')}
      >
        {copyState === 'ci' ? 'Copied' : 'Copy example'}
      </button>
    </div>

    <div class="reaction-panel">
      <div>
        <p class="eyebrow">No comment needed</p>
        <h4>React to the pull request description</h4>
        <p>Reactions on comments do not trigger commands</p>
      </div>
      <dl class="reactions">
        <div>
          <dt>👍</dt>
          <dd>Approve</dd>
        </div>
        <div>
          <dt>🚀</dt>
          <dd>Merge</dd>
        </div>
        <div>
          <dt>❤️</dt>
          <dd>Cleanup</dd>
        </div>
      </dl>
    </div>
  </section>

  <section class="help-section" id="help-configuration" aria-labelledby="help-config-title">
    <div class="section-heading">
      <div>
        <p class="eyebrow">Configuration</p>
        <h3 id="help-config-title">Start broad, override only what differs</h3>
      </div>
    </div>

    <ol class="precedence" aria-label="Configuration precedence from lowest to highest">
      <li>
        <div class="precedence-row">
          <span class="precedence-number mono">1</span>
          <div><strong>Service defaults</strong><small>Baseline behavior</small></div>
        </div>
      </li>
      <li>
        <div class="precedence-row">
          <span class="precedence-number mono">2</span>
          <div><strong>Account settings</strong><small>Applies to every repository</small></div>
        </div>
      </li>
      <li>
        <div class="precedence-row">
          <span class="precedence-number mono">3</span>
          <div>
            <strong>Repository file</strong><small><code>.github/smyklot.yaml</code></small>
          </div>
        </div>
      </li>
      <li class="highest">
        <div class="precedence-row">
          <span class="precedence-number mono">4</span>
          <div><strong>Repository settings</strong><small>Final repository override</small></div>
          <span class="precedence-badge mono">HIGHEST</span>
        </div>
      </li>
    </ol>

    <div class="config-notes">
      <article>
        <h4>Default means inherited</h4>
        <p>
          Choose Default to follow the next lower layer. Editing an inherited field creates an
          override automatically
        </p>
      </article>
      <article>
        <h4>Invalid files stop commands</h4>
        <p>
          A malformed repository file fails closed. Fix it or use the audited bypass control in
          repository settings
        </p>
      </article>
      <article>
        <h4>All commands start enabled</h4>
        <p>Deselect commands only when you want Smyklot to ignore them</p>
      </article>
    </div>
  </section>

  <section class="help-section" id="help-troubleshooting" aria-labelledby="help-trouble-title">
    <div class="section-heading">
      <div>
        <p class="eyebrow">Troubleshooting</p>
        <h3 id="help-trouble-title">Resolve the common blockers</h3>
      </div>
    </div>

    <div class="troubleshooting">
      <details>
        <summary>Smyklot does not respond</summary>
        <div>
          Confirm that the repository is On, the GitHub App is installed, the command is allowed,
          and
          <code>.github/smyklot.yaml</code> is valid. Check History → Failures for the delivery
        </div>
      </details>
      <details>
        <summary>Smyklot says you are not an owner</summary>
        <div>
          Add your exact GitHub handle to the global <code>*</code> rule in
          <code>.github/CODEOWNERS</code> on the default branch. Path-specific rules do not grant Smyklot
          access
        </div>
      </details>
      <details>
        <summary>A merge waits longer than expected</summary>
        <div>
          Open the pull request checks and identify the pending or failed check. Commands with
          <code>after required checks</code> ignore optional checks; <code>after CI</code> waits for all
          checks
        </div>
      </details>
      <details>
        <summary>A command works in one repository but not another</summary>
        <div>
          Compare repository state, file status, Allowed, Prefix, and Aliases. Repository settings
          override both account settings and the repository file
        </div>
      </details>
    </div>
  </section>

  <p class="copy-status visually-hidden" aria-live="polite">
    {copyState === 'failed'
      ? 'Could not copy the command'
      : copyState === null
        ? ''
        : 'Command copied'}
  </p>
</article>

<style>
  .help-page {
    display: grid;
    gap: 0.75rem;
  }

  .help-hero,
  .help-section {
    background: var(--strip);
    border: 1px solid var(--rule);
    border-radius: var(--r-strip);
    box-shadow: 0 4px 16px var(--shadow);
  }

  .help-hero-shell {
    isolation: isolate;
    margin-top: 0.25rem;
    position: relative;
  }

  .help-hero-shell::before {
    background: var(--spectrum);
    border-radius: var(--r-strip);
    bottom: 0.25rem;
    content: '';
    left: 0;
    pointer-events: none;
    position: absolute;
    right: 0;
    top: -0.25rem;
    z-index: 0;
  }

  .help-hero {
    align-items: center;
    display: grid;
    gap: 1.25rem;
    grid-template-columns: minmax(0, 1fr) minmax(10rem, 12rem);
    padding: 1rem 1.25rem;
    position: relative;
    z-index: 1;
  }

  h2,
  h3,
  h4,
  p {
    margin: 0;
  }

  h2 {
    font-size: clamp(1.5rem, 3vw, 2rem);
    line-height: 1.08;
    max-width: 42rem;
  }

  h3 {
    font-size: 1.125rem;
    line-height: 1.2;
  }

  h4 {
    font-size: 0.875rem;
    line-height: 1.25;
  }

  .hero-copy {
    display: grid;
    gap: 0.5rem;
  }

  .hero-copy > p:last-child {
    color: var(--dim);
    max-width: 46rem;
  }

  .hero-example {
    align-items: center;
    background: var(--well);
    border: 1px solid var(--rule);
    border-radius: var(--r-well);
    display: grid;
    gap: 0.75rem;
    grid-template-columns: 1fr auto;
    padding: 0.625rem;
  }

  .hero-example code {
    background: transparent;
    color: var(--text);
    font-size: 1rem;
    padding: 0;
  }

  .copy-button,
  .empty-search button {
    background: var(--admin);
    border: 1px solid var(--admin);
    border-radius: var(--r-ctl);
    color: var(--on-admin);
    font-size: 0.6875rem;
    font-weight: 750;
    height: 1.75rem;
    padding: 0 0.75rem;
  }

  .copy-button:hover,
  .empty-search button:hover {
    filter: brightness(1.08);
  }

  .topic-nav {
    align-items: center;
    background: var(--well);
    border: 1px solid var(--rule);
    border-radius: var(--r-ctl);
    display: flex;
    gap: 0.25rem;
    overflow-x: auto;
    padding: 0.25rem;
  }

  .topic-nav a {
    border-radius: calc(var(--r-ctl) - 2px);
    color: var(--dim);
    font-size: 0.75rem;
    font-weight: 650;
    padding: 0.45rem 0.75rem;
    text-decoration: none;
    white-space: nowrap;
  }

  .topic-nav a:hover,
  .topic-nav a:focus-visible {
    background: var(--strip-lift);
    color: var(--text);
  }

  .help-section {
    padding: 1.25rem;
    scroll-margin-top: 1rem;
  }

  .section-heading {
    align-items: start;
    display: flex;
    gap: 1rem;
    justify-content: space-between;
    margin-bottom: 0.875rem;
  }

  .section-heading > div:first-child {
    display: grid;
    gap: 0.45rem;
  }

  .quick-start-layout {
    display: grid;
    gap: 0.75rem;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .quick-panel {
    background: var(--rule);
    border: 1px solid var(--rule);
    border-radius: var(--r-well);
    display: grid;
    gap: 1px;
    grid-template-rows: auto 1fr;
    overflow: hidden;
  }

  .quick-panel-heading {
    align-items: center;
    background: var(--well);
    display: flex;
    justify-content: space-between;
    min-height: 2.5rem;
    padding: 0.5rem 0.75rem;
  }

  .quick-panel-heading h4 {
    font-size: 0.8125rem;
  }

  .quick-panel-heading span {
    color: var(--dim);
    font-size: 0.5625rem;
    letter-spacing: 0.1em;
  }

  .steps,
  .invocation-grid {
    display: grid;
    gap: 1px;
    grid-template-columns: 1fr;
    grid-template-rows: repeat(3, minmax(0, 1fr));
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .steps li,
  .invocation-card {
    align-items: center;
    background: var(--strip-lift);
    border: 0;
    border-radius: 0;
    display: grid;
    gap: 0.75rem;
    grid-template-columns: auto minmax(0, 1fr);
    min-height: 4.25rem;
    padding: 0.625rem 0.75rem;
  }

  .step-number {
    color: var(--accent);
    font-size: 0.625rem;
    font-weight: 800;
  }

  .steps strong,
  .method-copy strong {
    display: block;
    font-size: 0.75rem;
    line-height: 1.25;
  }

  .steps p,
  .method-copy p {
    margin-top: 0.2rem;
  }

  .steps p,
  .invocation-card p,
  .config-notes p,
  .reaction-panel p,
  .ci-callout p {
    color: var(--dim);
    font-size: 0.75rem;
    line-height: 1.45;
  }

  .invocation-card {
    grid-template-columns: minmax(0, 1fr) auto;
  }

  .invocation-card.recommended {
    background: var(--strip-lift);
    box-shadow: inset 3px 0 var(--accent);
  }

  .invocation-card code {
    justify-self: end;
    white-space: nowrap;
  }

  .command-heading {
    align-items: end;
  }

  .command-search {
    align-items: center;
    background: var(--input-surface);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    display: flex;
    height: var(--control-height);
    min-width: min(18rem, 100%);
    padding: 0 0.625rem;
  }

  .command-search svg {
    fill: none;
    flex: none;
    height: 1rem;
    stroke: var(--dim);
    stroke-linecap: round;
    stroke-width: 1.5;
    width: 1rem;
  }

  .command-search input {
    background: transparent;
    border: 0;
    color: var(--text);
    height: 100%;
    min-width: 0;
    outline: 0;
    padding: 0 0.5rem;
    width: 100%;
  }

  .command-search:focus-within {
    outline: 2px solid var(--brand);
    outline-offset: 2px;
  }

  .command-table-wrap {
    border: 1px solid var(--rule);
    border-radius: var(--r-well);
    overflow-x: auto;
  }

  .command-table {
    border-collapse: collapse;
    min-width: 38rem;
    table-layout: auto;
    width: 100%;
  }

  .command-column {
    width: 32%;
  }

  .example-column {
    width: 1%;
  }

  .command-table th,
  .command-table td {
    text-align: left;
  }

  .command-table thead th {
    background: var(--well);
    color: var(--dim);
    font: 650 0.5625rem/1 var(--mono);
    letter-spacing: 0.1em;
    padding: 0.625rem 0.75rem;
    text-transform: uppercase;
  }

  .command-table thead th:last-child {
    text-align: right;
  }

  .command-table tbody th,
  .command-table tbody td {
    background: var(--strip-lift);
    border-top: 1px solid var(--rule);
    padding: 0.625rem 0.75rem;
    vertical-align: middle;
  }

  .command-table tbody th {
    font-weight: 400;
  }

  .command-table tbody td {
    color: var(--dim);
    font-size: 0.6875rem;
  }

  .command-name {
    color: var(--text);
    font: 700 0.8125rem/1.2 var(--mono);
  }

  .command-identity {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: 0.25rem 0.75rem;
  }

  .command-table .command-example-cell {
    text-align: right;
    white-space: nowrap;
  }

  .command-example {
    align-items: center;
    background: var(--well);
    border: 1px solid var(--rule);
    border-radius: var(--r-ctl);
    color: var(--text);
    display: inline-flex;
    height: 2rem;
    justify-content: center;
    overflow: hidden;
    padding: 0 0.75rem;
    position: relative;
    transition:
      background-color 120ms ease-out,
      transform 90ms ease-out;
  }

  .command-example code {
    background: transparent;
    padding: 0;
  }

  .command-copy-overlay {
    align-items: center;
    background: var(--admin);
    color: var(--on-admin);
    display: flex;
    font: 650 0.625rem/1 var(--sans);
    gap: 0.35rem;
    inset: 0;
    justify-content: center;
    opacity: 0;
    position: absolute;
    transform: scale(0.97);
    transition:
      opacity 120ms ease-out,
      transform 120ms ease-out;
  }

  .command-copy-overlay svg {
    fill: none;
    height: 0.875rem;
    stroke: currentColor;
    stroke-linecap: round;
    stroke-linejoin: round;
    stroke-width: 1.5;
    width: 0.875rem;
  }

  .command-example:hover .command-copy-overlay,
  .command-example:focus-visible .command-copy-overlay,
  .command-copy-overlay.visible {
    opacity: 1;
    transform: scale(1);
  }

  .command-example:hover {
    background: var(--control-surface);
  }

  .command-example:active {
    background: var(--input-surface);
    transform: translateY(1px);
  }

  .command-example:focus-visible {
    outline: 2px solid var(--brand);
    outline-offset: 2px;
  }

  .aliases {
    color: var(--dim);
    font-size: 0.625rem;
  }

  .empty-search {
    align-items: center;
    background: var(--well);
    border-radius: var(--r-well);
    display: flex;
    gap: 1rem;
    justify-content: space-between;
    padding: 1rem;
  }

  .ci-callout {
    align-items: center;
    background: var(--signal-tint);
    border-radius: var(--r-well);
    display: grid;
    gap: 0.875rem;
    grid-template-columns: auto minmax(0, 1fr) auto;
    margin-top: 1rem;
    padding: 1rem;
  }

  .ci-icon {
    align-items: center;
    background: var(--well);
    border-radius: 50%;
    color: var(--signal);
    display: flex;
    height: 2.25rem;
    justify-content: center;
    width: 2.25rem;
  }

  .ci-icon svg {
    fill: none;
    height: 1.25rem;
    stroke: currentColor;
    stroke-linecap: round;
    stroke-linejoin: round;
    stroke-width: 1.4;
    width: 1.25rem;
  }

  .ci-callout h4,
  .reaction-panel h4 {
    margin-bottom: 0.25rem;
  }

  .reaction-panel {
    align-items: center;
    background: var(--well);
    border: 1px solid var(--rule);
    border-radius: var(--r-well);
    display: flex;
    flex-wrap: wrap;
    gap: 1rem;
    justify-content: space-between;
    margin-top: 0.75rem;
    padding: 1rem;
  }

  .reaction-panel > div:first-child {
    display: grid;
    gap: 0.35rem;
  }

  .reactions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    margin: 0;
  }

  .reactions div {
    align-items: center;
    background: var(--strip-lift);
    border-radius: var(--r-ctl);
    display: flex;
    gap: 0.4rem;
    padding: 0.45rem 0.65rem;
  }

  .reactions dt {
    font-size: 1rem;
  }

  .reactions dd {
    color: var(--dim);
    font-size: 0.6875rem;
    margin: 0;
  }

  .precedence {
    background: var(--rule);
    border: 1px solid var(--rule);
    border-radius: var(--r-well);
    display: grid;
    gap: 1px;
    list-style: none;
    margin: 0;
    overflow: visible;
    padding: 0;
  }

  .precedence li {
    isolation: isolate;
    position: relative;
  }

  .precedence-row {
    align-items: center;
    background: var(--strip-lift);
    display: grid;
    gap: 0.75rem;
    grid-template-columns: 1.75rem minmax(0, 1fr) auto;
    min-height: 3.5rem;
    padding: 0.625rem 0.75rem;
    position: relative;
    z-index: 1;
  }

  .precedence li:first-child .precedence-row {
    border-radius: calc(var(--r-well) - 1px) calc(var(--r-well) - 1px) 0 0;
  }

  .precedence li:last-child .precedence-row {
    border-radius: 0 0 calc(var(--r-well) - 1px) calc(var(--r-well) - 1px);
  }

  .precedence li.highest::before {
    background: var(--spectrum-vertical);
    border-radius: 0 0 0 var(--r-well);
    bottom: 0;
    content: '';
    left: -0.25rem;
    pointer-events: none;
    position: absolute;
    top: 0;
    width: 0.5rem;
    z-index: 0;
  }

  .precedence-number {
    align-items: center;
    background: var(--well);
    border: 1px solid var(--rule);
    border-radius: 50%;
    color: var(--accent);
    display: inline-flex;
    font-size: 0.625rem;
    height: 1.5rem;
    justify-content: center;
    width: 1.5rem;
  }

  .precedence-row > div {
    align-items: baseline;
    display: flex;
    flex-wrap: wrap;
    gap: 0.25rem 0.75rem;
  }

  .precedence strong {
    font-size: 0.75rem;
  }

  .precedence small {
    color: var(--dim);
    font-size: 0.625rem;
  }

  .precedence-badge {
    color: var(--accent);
    font-size: 0.5625rem;
    letter-spacing: 0.1em;
  }

  .config-notes {
    display: grid;
    gap: 0.75rem;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    margin-top: 0.75rem;
  }

  .config-notes article {
    border-top: 1px solid var(--rule);
    padding: 0.875rem 0.25rem 0;
  }

  .config-notes h4 {
    margin-bottom: 0.35rem;
  }

  .troubleshooting {
    border: 1px solid var(--rule);
    border-radius: var(--r-well);
    overflow: hidden;
  }

  .troubleshooting details + details {
    border-top: 1px solid var(--rule);
  }

  .troubleshooting summary {
    align-items: center;
    background: var(--strip-lift);
    cursor: pointer;
    display: flex;
    font-size: 0.8125rem;
    font-weight: 700;
    justify-content: space-between;
    list-style: none;
    min-height: 3rem;
    padding: 0.65rem 1rem;
    transition:
      background-color 120ms ease-out,
      color 120ms ease-out;
  }

  .troubleshooting summary:hover {
    background: var(--control-surface);
    color: var(--text);
  }

  .troubleshooting summary:active {
    background: var(--input-surface);
    color: var(--accent);
  }

  .troubleshooting summary:focus-visible {
    outline: 2px solid var(--brand);
    outline-offset: -2px;
  }

  .troubleshooting details[open] summary {
    background: var(--well);
  }

  .troubleshooting summary::-webkit-details-marker {
    display: none;
  }

  .troubleshooting summary::after {
    color: var(--accent);
    content: '+';
    font: 700 1rem/1 var(--mono);
  }

  .troubleshooting details[open] summary::after {
    content: '−';
  }

  .troubleshooting details > div {
    color: var(--dim);
    font-size: 0.75rem;
    line-height: 1.5;
    padding: 0.875rem 1rem 1rem;
  }

  @media (max-width: 52rem) {
    .command-heading {
      align-items: stretch;
      grid-template-columns: 1fr;
    }

    .command-heading {
      display: grid;
    }

    .command-search {
      width: 100%;
    }

    .reaction-panel {
      display: grid;
      grid-template-columns: 1fr;
    }

    .reactions {
      display: grid;
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }
  }

  @media (max-width: 38rem) {
    .help-hero,
    .help-section {
      padding: 1rem;
    }

    .help-hero,
    .quick-start-layout,
    .config-notes {
      grid-template-columns: 1fr;
    }

    .hero-example {
      min-width: 0;
    }

    .ci-callout {
      grid-template-columns: auto minmax(0, 1fr);
    }

    .callout-copy {
      grid-column: 1 / -1;
      width: 100%;
    }

    .reactions div {
      justify-content: center;
    }
  }
</style>

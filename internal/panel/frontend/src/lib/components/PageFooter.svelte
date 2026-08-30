<script lang="ts">
  import type { PanelBuild } from '../base';

  const { build }: { build: PanelBuild } = $props();
</script>

<!--
@component
What is running, said once at the bottom of the page: which build, and which service it
is talking to.

It renders nothing at all when it knows neither. An empty footer is a rule the page
still pays for in space and a reader still has to skip past, and a panel that cannot
say what it is running is better off not raising the subject.
-->

{#if build.version !== null || build.serviceHost !== null}
  <footer class="foot">
    <span class="foot-mark" aria-hidden="true"></span>
    <span class="foot-name band-trim">Panel</span>
    {#if build.version !== null}
      <span class="foot-env band-trim">{build.version}</span>
    {/if}
    <span class="foot-spacer"></span>
    {#if build.serviceHost !== null}
      <span class="foot-host mono band-trim">{build.serviceHost}</span>
    {/if}
  </footer>
{/if}

<style>
  /* Every text box is trimmed to its glyph bounds (cap height to baseline),
     so align-items centers the items against each other visually — no
     per-item nudges. Browsers without text-box fall back to line-height 1,
     which is close but keeps the font's descender bias. */
  .foot {
    align-items: center;
    border-top: 1px solid var(--border-subtle);
    color: var(--text-muted);
    display: flex;
    flex-wrap: wrap;
    font-size: var(--font-size-compact);
    gap: 0.625rem;
    line-height: var(--leading-flat);
    margin-top: 0.75rem;
    padding-top: 0.875rem;
  }

  .foot-mark {
    background: var(--brand-action);
    border-radius: 50%;
    flex: none;
    height: 8px;
    width: 8px;
  }

  .foot-name {
    color: var(--text-secondary);
    font-weight: 700;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }

  .foot-env {
    background: var(--brand-action-tint);
    border-radius: var(--r-chip);
    color: var(--brand-action-text);
    font: 600 0.65625rem / var(--leading-flat) var(--sans);
    letter-spacing: 0.05em;
    padding: 4px 8px;
    text-transform: uppercase;
  }

  .foot-spacer {
    flex: 1;
  }

  .foot-host {
    font-size: var(--font-size-compact);
    line-height: var(--leading-flat);
    overflow-wrap: anywhere;
  }
</style>

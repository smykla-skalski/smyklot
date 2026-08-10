<script lang="ts">
  import type { PanelBuild } from '../lib/base';

  const { build }: { build: PanelBuild } = $props();
</script>

{#if build.version !== null || build.serviceHost !== null}
  <footer class="foot">
    <span class="foot-mark" aria-hidden="true"></span>
    <span class="foot-name">Panel</span>
    {#if build.version !== null}
      <span class="foot-env">{build.version}</span>
    {/if}
    <span class="foot-spacer"></span>
    {#if build.serviceHost !== null}
      <span class="foot-host mono">{build.serviceHost}</span>
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
    border-top: 1px solid var(--rule);
    color: var(--dim);
    display: flex;
    flex-wrap: wrap;
    font-size: var(--font-size-compact);
    gap: 0.625rem;
    line-height: 1;
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
    text-box: trim-both cap alphabetic;
    text-transform: uppercase;
  }

  .foot-env {
    background: var(--brand-action-tint);
    border-radius: var(--r-chip);
    color: var(--brand-action-text);
    font: 600 0.65625rem / 1 var(--sans);
    letter-spacing: 0.05em;
    padding: 4px 8px;
    text-box: trim-both cap alphabetic;
    text-transform: uppercase;
  }

  .foot-spacer {
    flex: 1;
  }

  .foot-host {
    font-size: var(--font-size-compact);
    line-height: 1;
    overflow-wrap: anywhere;
    text-box: trim-both cap alphabetic;
  }
</style>

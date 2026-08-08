<script lang="ts">
  import type { PanelBuild } from '../lib/base';

  const { build }: { build: PanelBuild } = $props();

  const versions = $derived(
    [{ label: 'Panel', value: build.version }].filter(
      (mark): mark is { label: string; value: string } => mark.value !== null,
    ),
  );
</script>

{#if versions.length > 0 || build.serviceHost !== null}
  <footer class="foot">
    <div class="brand-rule brand-rule-close" aria-hidden="true"></div>
    <div class="marks">
      <dl class="versions">
        {#each versions as mark (mark.label)}
          <div class="mark">
            <dt>{mark.label}</dt>
            <dd class="mono">{mark.value}</dd>
          </div>
        {/each}
      </dl>

      {#if build.serviceHost !== null}
        <p class="host mono">{build.serviceHost}</p>
      {/if}
    </div>
  </footer>
{/if}

<style>
  .foot {
    margin-top: 1.75rem;
  }

  .marks {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem 1.5rem;
    justify-content: space-between;
    padding: 0.625rem 0.125rem 0;
  }

  .versions {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem 0.55rem;
    margin: 0;
    min-width: 0;
  }

  .mark {
    align-items: baseline;
    display: flex;
    gap: 0.5rem;
  }

  dt {
    color: var(--dim);
    font: 600 0.625rem/1.4 var(--mono);
    letter-spacing: 0.13em;
    text-transform: uppercase;
  }

  dd,
  .host {
    color: var(--dim);
    font-size: 0.6875rem;
    line-height: 1.4;
    margin: 0;
    overflow-wrap: anywhere;
  }
</style>

<script lang="ts">
  import type { PanelBuild } from '../base';

  const { build }: { build: PanelBuild } = $props();
</script>

<!--
@component
Known build and service details, quietly grouped at the bottom of existing navigation.
It renders nothing when both values are unknown. Long values stay available as full
accessible text and native tooltips while the visible columns remain bounded by the rail.
-->
{#if build.version !== null || build.serviceHost !== null}
  <div class="build-info" role="group" aria-label="Build information">
    <dl>
      {#if build.version !== null}
        <div>
          <dt>Panel</dt>
          <dd title={build.version}>{build.version}</dd>
        </div>
      {/if}
      {#if build.serviceHost !== null}
        <div>
          <dt>Service</dt>
          <dd title={build.serviceHost}>{build.serviceHost}</dd>
        </div>
      {/if}
    </dl>
  </div>
{/if}

<style>
  .build-info {
    color: var(--sidebar-text-muted);
    font-size: var(--font-size-compact);
    line-height: var(--row-copy-leading);
    min-inline-size: 0;
  }

  dl {
    display: grid;
    gap: var(--space-2);
    grid-template-columns: max-content minmax(0, 1fr);
    margin: 0;
  }

  dl > div {
    display: grid;
    gap: var(--space-2);
    grid-column: 1 / -1;
    grid-template-columns: subgrid;
    min-inline-size: 0;
  }

  dt,
  dd {
    margin: 0;
    text-box: trim-both cap alphabetic;
  }

  dd {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>

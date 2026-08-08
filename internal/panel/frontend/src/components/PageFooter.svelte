<script lang="ts">
  import type { PanelBuild } from '../lib/base';

  const {
    build,
    daemonVersion,
  }: {
    build: PanelBuild;
    /**
     * Separate from `build` because it arrives from a different place: the page
     * states its own version, while the daemon's comes back with the pairing
     * list. `null` until that read lands, and for anyone signed out.
     */
    daemonVersion: string | null;
  } = $props();

  /**
   * An equipment plate rather than a sign-off. The versions read as one group
   * because they are compared against each other when something is out of step;
   * the host is the separate fact, and it is the one worth being sure of before
   * minting a credential against it.
   */
  const versions = $derived(
    [
      { label: 'Panel', value: build.version },
      { label: 'Daemon', value: daemonVersion },
    ].filter((mark): mark is { label: string; value: string } => mark.value !== null),
  );
</script>

{#if versions.length > 0 || build.daemonHost !== null}
  <footer class="foot">
    <div class="beam beam-close" aria-hidden="true"></div>
    <div class="marks">
      <dl class="versions">
        {#each versions as mark, index (mark.label)}
          {#if index > 0}
            <span class="pip" aria-hidden="true"></span>
          {/if}
          <div class="mark">
            <dt>{mark.label}</dt>
            <dd class="mono">{mark.value}</dd>
          </div>
        {/each}
      </dl>

      {#if build.daemonHost !== null}
        <p class="host mono">{build.daemonHost}</p>
      {/if}
    </div>
  </footer>
{/if}

<style>
  .foot {
    margin-top: 1.75rem;
  }

  /* Not `.plate`: that is the page's card surface, and a footer wearing it
     would grow a raised slab under one line of small print. */
  .marks {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem 1.5rem;
    /* Pushed apart so the versions and the host read as separate facts, and so
       neither moves when the other's value changes length. */
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

  /* Drawn rather than typed: a middot sits on the x-height band, and both sides
     of it here are capitals, so a glyph would ride visibly low between them. */
  .pip {
    background: var(--dim);
    border-radius: 50%;
    flex: none;
    height: 3px;
    opacity: 0.6;
    width: 3px;
  }

  dt {
    color: var(--dim);
    font: 600 0.625rem/1.4 var(--mono);
    letter-spacing: 0.13em;
    text-transform: uppercase;
  }

  /* Left in its own case: a host is read back against DNS and a version against
     a tag, and neither survives being shouted. */
  dd,
  .host {
    color: var(--dim);
    font-size: 0.6875rem;
    line-height: 1.4;
    margin: 0;
    overflow-wrap: anywhere;
  }
</style>

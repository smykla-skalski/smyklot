<script lang="ts">
  import type { Snippet } from 'svelte';

  /** How much weight the plate carries; `lead` is the one the page is here for. */
  export type PlateTone = 'plain' | 'lead' | 'alarm';

  const {
    label,
    tone = 'plain',
    status,
    children,
  }: {
    label: string;
    tone?: PlateTone;
    /** Right-hand side of the header: the plate's current state, if it has one. */
    status?: Snippet;
    children?: Snippet;
  } = $props();
</script>

<section class="plate plate-{tone}" class:plate-header-only={children === undefined}>
  <header class="plate-head">
    <h2 class="eyebrow">{label}</h2>
    {#if status !== undefined}
      {@render status()}
    {/if}
  </header>
  {#if children !== undefined}
    <div class="plate-body">
      {@render children()}
    </div>
  {/if}
</section>

<style>
  h2 {
    color: var(--text-primary);
    font-size: var(--font-size-body);
    font-weight: 650;
    letter-spacing: 0;
    text-transform: none;
  }

  /* A plate with a body is a full card: its title earns a size step up. */
  section:not(.plate-header-only) h2 {
    font-size: 1.0625rem;
    font-weight: 700;
  }
</style>

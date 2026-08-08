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
    children: Snippet;
  } = $props();
</script>

<section class="plate plate-{tone}">
  <header class="plate-head">
    <h2 class="eyebrow">{label}</h2>
    {#if status !== undefined}
      {@render status()}
    {/if}
  </header>
  <div class="plate-body">
    {@render children()}
  </div>
</section>

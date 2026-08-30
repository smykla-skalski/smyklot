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

<!--
@component
A titled ground for one subject, with its state said on the same line as its name.
The header is always drawn, the body optional: a plate with nothing in it is a subject
that has nothing to report yet, which is a state worth showing rather than a hole to
hide.

The tone colours the ground, not the words. It says what KIND of thing this is - a
plain fact, something needing attention - and never how urgent it is, because a plate
is a place rather than a message. Something the reader must act on now is a `Callout`
inside it.
-->

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

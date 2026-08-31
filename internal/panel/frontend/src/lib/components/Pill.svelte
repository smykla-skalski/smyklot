<script module lang="ts">
  /**
   * What the pill reports, never what colour it is.
   *
   * `bare` is a standing with no outcome attached - a role beside a name; `role` is the
   * one role worth tinting, the standing somebody holds over everyone else's;
   * `success`, `warning` and `danger` are outcomes; `info` and `neutral` are quiet
   * qualifiers.
   */
  export type PillTone = 'bare' | 'role' | 'success' | 'warning' | 'danger' | 'info' | 'neutral';
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';

  const { tone = 'bare', children }: { tone?: PillTone; children: Snippet } = $props();

  const classes = $derived(
    ['pill', tone === 'bare' ? '' : `pill-${tone}`].filter(Boolean).join(' '),
  );
</script>

<!--
@component
A standing: the role somebody holds, or the state a thing is in. Not a `Chip`, which is
a *value* - a label name, a pattern, a count - and wears the rounded-rect shape so the
two never read as the same object.

The label is always wrapped. A pill is a flex container, so bare text sits in an
anonymous box no selector reaches and the `text-box` trim never touches it - the same
reason `Button` wraps its own. Both live in `app.css`, so this file has no `<style>`.
-->

<span class={classes}><span class="pill-label">{@render children()}</span></span>

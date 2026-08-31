<script lang="ts">
  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';
  import PageHeader from './PageHeader.svelte';

  const {
    title,
    subtitle,
    headingId = 'root-page-heading',
    children,
  }: {
    title: string;
    subtitle?: string;
    headingId?: string;
    children?: Snippet;
  } = $props();
</script>

<!--
@component
The Root console's page header: `PageHeader`, with the console's own name.

It used to open every page with "Root mode · Super Root" and a pill reading
"Application scope" - three ways of saying where you are, on a purple shell
whose sidebar is titled Operations and whose every page is the console's. The
name is the eyebrow now, and the shield is the same mark the sidebar wears.

The anatomy is shared - it used to be copied, 72 of ~81 CSS lines identical to
`PageHeader`'s, comments and all - so what is genuinely the console's is this
one line, and that is all this file is.
-->

<PageHeader id={headingId} {title} description={subtitle} actions={children}>
  {#snippet kicker()}
    <Icon name="shield" size="xs" /><span class="cap-trim">Operations console</span>
  {/snippet}
</PageHeader>

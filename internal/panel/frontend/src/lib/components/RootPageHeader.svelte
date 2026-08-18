<script lang="ts">
  import type { Snippet } from 'svelte';

  import PageHeader from './PageHeader.svelte';
  import StatusPill from './StatusPill.svelte';

  /**
   * The Root console's page header: `PageHeader`, plus the line that says whose
   * authority the page is under.
   *
   * The anatomy is shared - it used to be copied, 72 of ~81 CSS lines identical to
   * `PanelHeader`'s, comments and all. What is genuinely the console's is this
   * kicker, so that is all this file is now.
   *
   * Scope is identity, so its pill sits on the kicker line; the action slot holds
   * only live status and real controls.
   */
  const {
    role,
    title,
    subtitle,
    headingId = 'root-page-heading',
    showScope = true,
    children,
  }: {
    role: string;
    title: string;
    subtitle: string;
    headingId?: string;
    showScope?: boolean;
    children?: Snippet;
  } = $props();
</script>

<PageHeader id={headingId} {title} description={subtitle} actions={children}>
  {#snippet kicker()}
    <span class="cap-trim">Root mode · {role}</span>
    {#if showScope}
      <StatusPill>Application scope</StatusPill>
    {/if}
  {/snippet}
</PageHeader>

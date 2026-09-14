<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import type { PanelApi } from '#lib/api.js';
  import Button from './Button.svelte';
  import FormError from './FormError.svelte';

  const { api, reload }: { api: Pick<PanelApi, 'fetchInstallation'>; reload: () => Promise<void> } =
    $props();
  const installation = createQuery(() => ({
    queryKey: ['installation'],
    queryFn: ({ signal }) => api.fetchInstallation(signal),
    staleTime: 5 * 60_000,
    retry: false,
  }));
</script>

<!--
@component
The next step for a signed-in reader without workspace access. GitHub owns
installation permissions. The panel keeps its return point and refresh action.
-->
<section class="installation-prompt" aria-label="Get workspace access">
  <h2>Connect your first workspace</h2>
  <p>
    Install the GitHub App on your account or organization, then return here and reload the panel.
  </p>
  {#if installation.isPending && installation.data === undefined}
    <p class="form-help" role="status">Loading installation link…</p>
  {:else if installation.isError}
    <FormError
      message="Could not load the installation link. Try again, or ask the person who runs this Smyklot service for the link."
    />
    <Button
      aria-disabled={installation.isFetching}
      onclick={() => {
        if (!installation.isFetching) void installation.refetch();
      }}>Retry installation link</Button
    >
  {:else if installation.data}
    <Button tone="signal" href={installation.data} target="_blank" rel="noopener noreferrer"
      >Install GitHub App</Button
    >
    <p class="form-help">
      Opens GitHub in a new tab. If you cannot install it, request approval there or ask your
      organization owner.
    </p>
  {:else}
    <p class="form-help">
      Installation is not available from this panel. Ask the person who runs this Smyklot service
      for an installation link.
    </p>
  {/if}
  <p class="form-help">Already installed? Ask a workspace owner to give you access.</p>
  <Button onclick={() => void reload()}>Reload panel</Button>
</section>

<style>
  .installation-prompt {
    display: grid;
    gap: var(--space-4);
    justify-items: center;
    text-align: center;
    max-width: var(--measure-note);
    margin-inline: auto;
  }
  h2 {
    margin: 0;
    font-size: var(--font-size-card-title);
  }
  p {
    margin: 0;
  }
</style>

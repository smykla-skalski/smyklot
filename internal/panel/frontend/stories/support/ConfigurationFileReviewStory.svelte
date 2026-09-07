<script lang="ts">
  import { onMount } from 'svelte';
  import ConfigurationFileReview from '#lib/components/ConfigurationFileReview.svelte';
  import Button from '#lib/components/Button.svelte';
  import type { ConfigurationReviewSource } from '../../src/lib/config-file-review.svelte';
  import {
    mockConfigFilePreview,
    type ConfigFileReviewVariant,
  } from '../../dev/config-file-review';

  const {
    variant = 'conflict',
    readOnly = false,
    hasDrafts = false,
  }: { variant?: ConfigFileReviewVariant; readOnly?: boolean; hasDrafts?: boolean } = $props();
  let inspector = $state<ConfigurationFileReview | null>(null);
  const repository = 'smykla-skalski/edge-proxy';
  const source = $derived<ConfigurationReviewSource>({
    identity: `story/${variant}/${readOnly}`,
    hasDrafts,
    canWrite: !readOnly,
    enabled: true,
    fileIgnored: false,
    preview: async () => mockConfigFilePreview(variant, repository, 'a'.repeat(64)),
    resolve: async () => ({ status: 'pending' }),
    onResolved: () => {},
  });
  onMount(() => inspector?.openReview());
</script>

<Button onclick={(event) => inspector?.openReview(event.currentTarget)}>Review file</Button>
<ConfigurationFileReview bind:this={inspector} {repository} {source} />

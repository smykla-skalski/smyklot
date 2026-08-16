<script lang="ts">
  import { page } from '$app/state';

  const message = page.error?.message ?? 'Something went wrong';
</script>

<svelte:head>
  <title>SMYKLOT</title>
</svelte:head>

<div style="padding: 2rem; font-family: system-ui, sans-serif; color: inherit;">
  {message}
</div>

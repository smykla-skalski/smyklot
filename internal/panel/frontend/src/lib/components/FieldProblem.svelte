<script lang="ts">
  const { id, message }: { id: string; message?: string | null } = $props();
</script>

{#if message}
  <span {id} class="field-problem">{message}</span>
{/if}

<style>
  .field-problem {
    display: block;
    overflow-wrap: anywhere;
    color: var(--danger);
    font-size: var(--font-size-compact);
  }
</style>

<script lang="ts">
  const { id, message }: { id: string; message?: string | null } = $props();
</script>

<!--
@component
An error beside the field that needs correction. The owning control references
this message with aria-describedby and exposes its invalid state. The caller
keeps the error attached to the rejected value and clears it after correction.

Use FormError for a submission-wide alert. This description does not announce
another alert when a persisted field error renders after navigation or reload.
No message means no element, so callers must omit the description reference too.
-->

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

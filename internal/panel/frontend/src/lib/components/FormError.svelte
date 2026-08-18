<script lang="ts">
  /**
   * What a form says when the server refused it.
   *
   * Twelve call sites wrote this line, and the part that matters is the part easiest
   * to leave off: `role="alert"`. Without it the message paints and nothing announces
   * it, so a reader who cannot see the form is told the submission simply did not
   * happen. It is the reason this is a component rather than a class.
   *
   * Rendered only when there is something to say, so callers stop writing the guard.
   */
  const { message }: { message: string | null | undefined } = $props();
</script>

{#if message !== null && message !== undefined && message !== ''}
  <p class="form-error" role="alert">{message}</p>
{/if}

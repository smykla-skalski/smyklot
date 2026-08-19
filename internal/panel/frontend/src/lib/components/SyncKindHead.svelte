<script lang="ts">
  /**
   * What one kind of sync is, and the one control that decides whether it runs.
   *
   * Every kind whose configuration is a document opens the same way: its name,
   * a line saying what it holds, a switch, and the same three answers to the
   * same three questions - did the last save fail, can this version read what
   * is stored, and is a permission missing. Written per kind they were kept in
   * step by hand, which is how one kind comes to answer a question the next one
   * has stopped asking.
   *
   * The switch is a switch rather than a saved choice because flipping it *is*
   * the act: it makes the kind eligible for planning and nothing more, so there
   * is nothing to hold back for a Save. Nothing reaches GitHub either way until
   * a plan is approved.
   */
  import FormError from './FormError.svelte';
  import Switch from './Switch.svelte';

  const {
    title,
    lead,
    noun,
    enabled,
    unreadable,
    unavailable = '',
    problem = null,
    readOnly,
    saving,
    onToggle,
  }: {
    title: string;
    lead: string;
    /**
     * What these are called, in the plural, lower case. It names the switch and
     * both notices, so a kind cannot spell itself one way in one of them and
     * another way in the next.
     */
    noun: string;
    enabled: boolean;
    unreadable: boolean;
    /**
     * What this kind needs and the installation has not granted, or empty.
     * Configuring is still allowed - configuring before granting is the ordinary
     * order - but a switch that is on while this is set changes nothing.
     */
    unavailable?: string;
    /** What went wrong saving this kind, which belongs beside it. */
    problem?: string | null;
    readOnly: boolean;
    saving: boolean;
    onToggle: (enabled: boolean) => void;
  } = $props();

  const helpId = $derived(`sync-${noun}-help`);
</script>

<div class="kind-head">
  <div class="kind-head-say">
    <h2 class="kind-title band-trim">{title}</h2>
    <p class="kind-lead" id={helpId}>{lead}</p>
  </div>
  <Switch
    label="Syncing"
    checked={enabled}
    disabled={saving || readOnly || unreadable}
    describedBy={helpId}
    onChange={onToggle}
  />
</div>

{#if problem !== null}
  <FormError message={problem} />
{/if}

{#if unreadable}
  <p class="kind-notice" role="alert">
    These {noun} are stored in a form this version of Smyklot cannot read, so they are not shown and nothing
    here can be changed. Nothing has been lost
  </p>
{/if}

<!-- Only while the switch is on, because that is when the difference shows: a
     kind nobody asked for is not waiting on anything. Bound to the switch
     rather than to what was saved, so somebody turning it on is told at once. -->
{#if unavailable !== '' && enabled}
  <p class="kind-notice" role="status">
    {unavailable}. Nothing here will be planned or changed until an owner grants it on the
    installation's page on GitHub. The {noun} below can still be configured
  </p>
{/if}

<style>
  .kind-head {
    align-items: start;
    display: flex;
    gap: var(--space-4);
    justify-content: space-between;
    margin-bottom: var(--space-4);
  }

  .kind-title {
    font-size: var(--font-size-title);
    font-weight: 600;
    margin: 0;
  }

  .kind-lead {
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
    margin: var(--space-2) 0 0;
    max-width: 66ch;
  }

  /* The switch keeps its line even where the sentence beside it wraps to three:
     it is read as the head's control, not as the last word of the paragraph. */
  .kind-head :global(.switch) {
    flex: none;
    margin-block-start: 0.1rem;
  }

  .kind-notice {
    background: var(--surface-inset);
    border-radius: var(--r-ctl);
    font-size: var(--font-size-meta);
    margin: 0 0 var(--space-4);
    padding: var(--space-2) var(--space-3);
  }

  @media (max-width: 40rem) {
    .kind-head {
      align-items: start;
      flex-direction: column;
    }
  }
</style>

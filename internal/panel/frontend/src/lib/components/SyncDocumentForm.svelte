<script lang="ts">
  /**
   * The shell every sync kind whose configuration is one document wears.
   *
   * Settings and rulesets ask the same four questions in the same order - did
   * the last save fail, can this version read what is stored, is a permission
   * missing, and is any of this switched on - and answer them in the same
   * words with the noun changed. The view above already keeps its state per
   * kind for this reason, and said why: three fields per kind is how the third
   * one comes to reuse the second one's by accident. Files are next.
   *
   * Presentation only. The kind that renders through this owns the state,
   * because it is the one that knows what a change to its own document is.
   */
  import type { Snippet } from 'svelte';

  import Plate from './Plate.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  /** The same two words the settings page puts on the same decision. */
  const SYNC_OPTIONS = [
    { value: 'enabled', label: 'Enabled' },
    { value: 'disabled', label: 'Disabled' },
  ] as const;

  const {
    heading,
    noun,
    lead,
    enabled,
    unreadable,
    unavailable = '',
    problem = null,
    readOnly,
    saving,
    changed,
    disabled,
    onToggle,
    onSave,
    children,
    actions,
  }: {
    heading: string;
    /**
     * What these are called, in the plural, lower case. It is the switch, the
     * save button, the unreadable message and the notice, so a kind cannot
     * spell itself one way in one of them and another way in the next.
     */
    noun: string;
    lead: string;
    /** Where the switch is now, which is the kind's draft rather than what is saved. */
    enabled: boolean;
    unreadable: boolean;
    /**
     * What this kind needs and the installation has not granted, or empty.
     * Saving is still allowed - configuring before granting is the ordinary
     * order - but a switch that is on while this is set changes nothing.
     */
    unavailable?: string;
    /**
     * What went wrong saving this kind, which belongs beside it. The forms on
     * this page save separately and none waits for another, so one shared
     * message is one form's failure wiped by the next form's click.
     */
    problem?: string | null;
    readOnly: boolean;
    saving: boolean;
    changed: boolean;
    disabled: boolean;
    onToggle: (enabled: boolean) => void;
    onSave: () => void;
    children: Snippet;
    /** Anything the kind offers beside Save, such as adding a row. */
    actions?: Snippet;
  } = $props();

  const helpId = $derived(`sync-${noun}-help`);
</script>

<Plate label={heading}>
  {#snippet status()}
    <SegmentedControl
      name="sync-{noun}-switch"
      label="Sync {noun}"
      descriptionId={helpId}
      options={SYNC_OPTIONS}
      value={enabled ? 'enabled' : 'disabled'}
      compact
      {disabled}
      onSelect={(selection) => onToggle(selection === 'enabled')}
    />
  {/snippet}

  <p class="form-lead" id={helpId}>{lead}</p>

  {#if problem !== null}
    <p class="form-error" role="alert">{problem}</p>
  {/if}

  {#if unreadable}
    <p class="form-notice" role="alert">
      These {noun} are stored in a form this version of Smyklot cannot read, so they are not shown and
      nothing here can be changed. Nothing has been lost
    </p>
  {/if}

  <!-- Only while the switch is on, because that is when the difference shows:
       a kind nobody asked for is not waiting on anything. Bound to the switch
       rather than to what was saved, so somebody turning it on is told before
       they press save rather than after. -->
  {#if unavailable !== '' && enabled}
    <p class="form-notice" role="status">
      {unavailable}. Nothing here will be planned or changed until an owner grants it on the
      installation's page on GitHub. The {noun} below can be saved in the meantime
    </p>
  {/if}

  {@render children()}

  {#if !readOnly}
    <div class="form-actions">
      {#if actions}
        {@render actions()}
      {/if}
      <button class="btn btn-signal" type="button" disabled={disabled || !changed} onclick={onSave}>
        {saving ? 'Saving' : `Save ${noun}`}
      </button>
      {#if changed}
        <p class="form-note">Nothing is changed on GitHub until a plan is approved</p>
      {/if}
    </div>
  {/if}
</Plate>

<style>
  .form-lead,
  .form-note {
    color: var(--dim);
    font-size: var(--font-size-meta);
    margin: 0;
    max-width: 60ch;
  }

  .form-notice {
    background: var(--surface-inset);
    border-radius: var(--r-ctl);
    font-size: var(--font-size-meta);
    margin: var(--space-3) 0 0;
    padding: var(--space-2) var(--space-3);
  }

  /* The global rule has no margin, because most of the places it appears sit in
     a gapped grid. This one sits in a plate's flow, under the line explaining
     the kind. */
  .form-error {
    margin: var(--space-3) 0 0;
  }

  .form-actions {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    margin-top: var(--space-5);
  }
</style>

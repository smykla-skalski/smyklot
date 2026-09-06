<script module lang="ts">
  export type SettingsDraftAttentionKind = 'inactive' | 'storage-problem';
</script>

<script lang="ts">
  import { SETTINGS_DRAFT_INACTIVITY_MINUTES } from '../settings-draft-attention';
  import Button from './Button.svelte';
  import Callout from './Callout.svelte';
  import Icon, { type IconName } from './Icon.svelte';

  const {
    kind,
    count = 0,
    problem = null,
    reviewHref,
    onDismiss,
  }: {
    kind: SettingsDraftAttentionKind;
    count?: number;
    problem?: string | null;
    reviewHref?: string;
    onDismiss: () => void;
  } = $props();

  const heading = $derived(
    kind === 'inactive' ? 'Unsaved settings need attention' : 'Unsaved settings may not survive',
  );
  const iconName = $derived<IconName>(kind === 'inactive' ? 'pending' : 'warning');
  const detail = $derived.by(() => {
    const settings = `${count} unsaved ${count === 1 ? 'setting' : 'settings'}`;
    if (kind === 'inactive') {
      return `This tab was out of view for at least ${SETTINGS_DRAFT_INACTIVITY_MINUTES} minutes. ${settings} ${count === 1 ? 'is' : 'are'} still here and not saved`;
    }
    return problem ?? 'Browser storage is unavailable. Unsaved changes will not survive';
  });
</script>

<!--
@component
The line that says a draft is waiting somewhere the reader is not looking. Settings
drafts survive navigation, so a page can be left with changes on it and nothing on
screen would otherwise say so.

Distinct from the save composer: that one is the bar on the page that owns the draft,
and this is the notice everywhere else. `reviewHref` is the way back to it.

`storage-problem` is the other kind - a draft that could not be kept in browser
storage. Both kinds use the shared pending-decision treatment; storage failure
retains warning typography and alert semantics without implying the draft is gone.
-->

<div class="settings-draft-attention" data-kind={kind}>
  <Callout
    decision
    tone={kind === 'storage-problem' ? 'warning' : 'quiet'}
    role={kind === 'storage-problem' ? 'alert' : 'status'}
    aria-live={kind === 'storage-problem' ? 'assertive' : 'polite'}
    aria-atomic="true"
  >
    {#snippet icon()}
      <Icon name={iconName} size="base" strokeWidth={2} />
    {/snippet}
    <div class="callout-copy">
      <strong>{heading}</strong>
      <span>{detail}</span>
    </div>
    {#snippet actions()}
      {#if reviewHref !== undefined && kind !== 'storage-problem'}
        <Button tone="signal" row href={reviewHref} onclick={onDismiss}>Review</Button>
      {/if}
      <Button tone="quiet" row onclick={onDismiss}>Dismiss</Button>
    {/snippet}
  </Callout>
</div>

<style>
  .settings-draft-attention {
    animation: attention-arrive var(--duration-fast) var(--ease-standard) both;
  }

  @keyframes attention-arrive {
    from {
      opacity: 0;
      transform: translateY(-0.5rem);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .settings-draft-attention {
      animation: none;
    }
  }
</style>

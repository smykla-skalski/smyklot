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

`storage-problem` is the other kind - a draft that could not be kept - and takes the
warning tone, because a draft the panel has lost is the one thing here a reader cannot
recover by going back.
-->

<div class="settings-draft-attention" data-kind={kind}>
  <Callout
    class="attention-surface"
    tone={kind === 'storage-problem' ? 'warning' : 'quiet'}
    role={kind === 'storage-problem' ? 'alert' : 'status'}
    aria-live={kind === 'storage-problem' ? 'assertive' : 'polite'}
    aria-atomic="true"
  >
    {#snippet icon()}
      <span class="attention-mark"><Icon name={iconName} size="base" strokeWidth={2} /></span>
    {/snippet}
    <div class="attention-copy">
      <strong>{heading}</strong>
      <span>{detail}</span>
    </div>
    <span class="attention-actions">
      {#if reviewHref !== undefined && kind !== 'storage-problem'}
        <Button tone="brand" row href={reviewHref} onclick={onDismiss}>Review</Button>
      {/if}
      <Button tone="quiet" row onclick={onDismiss}>Dismiss</Button>
    </span>
  </Callout>
</div>

<style>
  .settings-draft-attention {
    animation: attention-arrive var(--duration-fast) var(--ease-standard) both;
  }

  .settings-draft-attention :global(.attention-surface) {
    -webkit-backdrop-filter: blur(18px) saturate(118%);
    backdrop-filter: blur(18px) saturate(118%);
    background: color-mix(in srgb, var(--surface-raised) 82%, transparent);
    border-color: color-mix(in srgb, var(--text-primary) 18%, transparent);
    box-shadow:
      var(--shadow-popover),
      inset 0 1px 0 color-mix(in srgb, white 12%, transparent);
  }

  .settings-draft-attention[data-kind='inactive'] :global(.attention-surface) {
    background: color-mix(in srgb, var(--brand-action-tint) 82%, transparent);
    border-color: color-mix(in srgb, var(--brand-action) 28%, transparent);
  }

  .settings-draft-attention[data-kind='storage-problem'] :global(.attention-surface) {
    background: color-mix(in srgb, var(--warning-tint) 84%, transparent);
    border-color: color-mix(in srgb, var(--warning) 34%, transparent);
  }

  .attention-mark {
    color: var(--text-secondary);
    display: inline-flex;
    flex: 0 0 auto;
  }

  .attention-copy {
    display: grid;
    flex: 1;
    gap: var(--space-1);
    min-width: 0;
  }

  .attention-copy strong {
    color: var(--text-primary);
  }

  .attention-actions {
    align-self: center;
    display: flex;
    flex: 0 0 auto;
    gap: var(--space-1);
  }

  @keyframes attention-arrive {
    from {
      opacity: 0;
      transform: translateY(-0.5rem);
    }
  }

  @media (max-width: 36rem) {
    .attention-actions {
      align-self: start;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .settings-draft-attention {
      animation: none;
    }
  }
</style>

<script lang="ts">
  import Callout from '#lib/components/Callout.svelte';
  import Icon from '#lib/components/Icon.svelte';
  import Button from '#lib/components/Button.svelte';
  import SettingsDraftAttention from '#lib/components/SettingsDraftAttention.svelte';
</script>

<div class="callout-matrix">
  {#each ['quiet', 'warning'] as const as tone (tone)}
    <section aria-label={tone}>
      <h2 class="card-title">{tone === 'quiet' ? 'Information' : 'Warning'}</h2>
      <Callout {tone} data-case={`${tone}-single`}>
        {#snippet icon()}<Icon name="warning" size="base" />{/snippet}
        <span>Review before continuing</span>
      </Callout>
      <Callout {tone} data-case={`${tone}-wrapped`}>
        {#snippet icon()}<Icon name="warning" size="md" />{/snippet}
        <span
          >This workspace is not yours. Every change is audited and every Owner receives a
          notification</span
        >
      </Callout>
      <Callout {tone} data-case={`${tone}-single-action`}>
        {#snippet icon()}<Icon name="warning" size="base" />{/snippet}
        <span>Review this request</span>
        {#snippet actions()}<Button tone="quiet">Review</Button>{/snippet}
      </Callout>
      <Callout {tone} data-case={`${tone}-heading`}>
        {#snippet icon()}<Icon name="warning" size="base" />{/snippet}
        <div class="callout-copy">
          <strong>Review this request</strong>
          <p>A new invitation reaches the same GitHub account and appears in the audit record</p>
        </div>
      </Callout>
      <Callout {tone} data-case={`${tone}-action`}>
        {#snippet icon()}<Icon name="warning" size="md" />{/snippet}
        <div class="callout-copy">
          <strong>Changes need attention</strong>
          <p>Review the current workspace before applying your saved draft</p>
        </div>
        {#snippet actions()}<Button tone="quiet">Review</Button>{/snippet}
      </Callout>
    </section>
  {/each}
  <section aria-label="Draft notices">
    <h2 class="card-title">Draft notices</h2>
    <SettingsDraftAttention
      kind="inactive"
      count={2}
      reviewHref="/workspace"
      onDismiss={() => {}}
    />
    <SettingsDraftAttention
      kind="storage-problem"
      count={2}
      problem="Browser storage is full. Unsaved workspace and repository settings will not survive closing this tab"
      onDismiss={() => {}}
    />
  </section>
</div>

<style>
  .callout-matrix {
    display: grid;
    gap: var(--space-6);
    padding: var(--space-4);
    max-width: 38rem;
    margin-inline: auto;
  }
  section {
    display: grid;
    gap: var(--space-4);
  }
</style>

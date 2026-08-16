<script lang="ts">
  import { Tabs } from 'bits-ui';

  const {
    label,
    value,
    options,
    onSelect,
  }: {
    label: string;
    value: string;
    options: ReadonlyArray<{ value: string; label: string; badge?: string | number }>;
    onSelect: (value: string) => void;
  } = $props();
</script>

<Tabs.Root class="navigation-tabs" {value} onValueChange={onSelect} activationMode="manual">
  <Tabs.List class="navigation-tab-list" aria-label={label}>
    {#each options as option (option.value)}
      <Tabs.Trigger class="navigation-tab" value={option.value}>
        <!-- Trimmed to its own band, so the equal padding above and below is the
             whole of what centres the word on the tab. Untrimmed it sat 0.34px
             high of its own surface, which is a device row at 2x. -->
        <span class="cap-trim">{option.label}</span>
        {#if option.badge !== undefined}<span class="navigation-badge">{option.badge}</span>{/if}
      </Tabs.Trigger>
    {/each}
  </Tabs.List>
</Tabs.Root>

<style>
  :global(.navigation-tabs) {
    display: inline-flex;
  }

  :global(.navigation-tab-list) {
    background: var(--segment-track);
    border: 1px solid var(--rule);
    border-radius: var(--r-ctl);
    display: inline-flex;
    gap: var(--control-inset);
    padding: var(--control-inset);
  }

  :global(.navigation-tab) {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: calc(var(--r-ctl) - var(--control-inset));
    color: var(--text-muted);
    display: inline-flex;
    font: 650 var(--font-size-meta) / 1 var(--sans);
    gap: 0.35rem;
    min-height: calc(var(--control-height-compact) - 2 * var(--control-inset));
    padding: 0 var(--space-3);
  }

  :global(.navigation-tab:hover) {
    background: var(--segment-hover);
    color: var(--text);
  }

  :global(.navigation-tab[data-state='active']) {
    background: var(--segment-thumb);
    box-shadow: var(--segment-shadow);
    color: var(--text-primary);
  }

  :global(.navigation-badge) {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
  }
</style>

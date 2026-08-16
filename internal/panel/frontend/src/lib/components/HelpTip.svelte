<script lang="ts">
  import { Tooltip } from 'bits-ui';
  import Icon from './Icon.svelte';

  const {
    id,
    label,
    text,
    align = 'end',
  }: {
    id: string;
    label: string;
    text: string;
    align?: 'start' | 'end';
  } = $props();
</script>

<span class="help-tip" class:align-start={align === 'start'}>
  <Tooltip.Provider delayDuration={250}>
    <Tooltip.Root>
      <Tooltip.Trigger class="help-trigger" aria-label={label}>
        <Icon name="info" size={14} strokeWidth={2} />
      </Tooltip.Trigger>
      <Tooltip.Portal to=".app-shell">
        <Tooltip.Content
          {id}
          class="help-content"
          side="top"
          {align}
          sideOffset={6}
          collisionPadding={8}
        >
          {text}
        </Tooltip.Content>
      </Tooltip.Portal>
    </Tooltip.Root>
  </Tooltip.Provider>
</span>

<style>
  .help-tip {
    display: inline-flex;
  }

  :global(.help-trigger) {
    background: transparent;
    border: 0;
    border-radius: var(--r-ctl);
    color: var(--dim);
    cursor: help;
    display: inline-grid;
    height: 1.125rem;
    padding: 0;
    place-items: center;
    width: 1.125rem;
  }

  :global(.help-trigger:hover),
  :global(.help-trigger:focus-visible) {
    color: var(--signal);
  }

  :global(.help-content) {
    background: var(--text-primary);
    border-radius: var(--radius-control);
    color: var(--surface);
    font-size: var(--font-size-meta);
    max-width: 18rem;
    padding: var(--space-2) var(--space-3);
    z-index: var(--layer-tooltip);
  }
</style>

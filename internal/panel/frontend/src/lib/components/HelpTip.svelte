<script lang="ts">
  import AppTooltip from './AppTooltip.svelte';
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

<!--
@component
The question mark beside a control, and the sentence it holds.

The tip itself is `AppTooltip`, which is the panel's one tooltip: this used
to wire up its own Bits UI tooltip and paint it in reverse - dark ground,
`var(--surface)` text - and `--surface` is a token nothing declares. An
unresolvable custom property is invalid at computed-value time, so the colour
fell back to the one it inherits, which on that ground is the ground: every
help tip in the panel has been a black rectangle with its sentence painted
inside it in black. Nobody had ever read one, so there is no look here to
keep, and one tooltip is better than two.
-->

<span class="help-tip" class:align-start={align === 'start'}>
  <AppTooltip {id} {text} {align}>
    {#snippet children(props)}
      <button {...props} type="button" class="help-trigger" aria-label={label}>
        <Icon name="info" size="sm" strokeWidth={2} />
      </button>
    {/snippet}
  </AppTooltip>
</span>

<style>
  .help-tip {
    display: inline-flex;
  }

  .help-trigger {
    background: transparent;
    border: 0;
    border-radius: var(--r-ctl);
    color: var(--text-muted);
    cursor: help;
    display: inline-grid;
    height: 1.125rem;
    padding: 0;
    place-items: center;
    width: 1.125rem;
  }

  .help-trigger:hover,
  .help-trigger:focus-visible {
    color: var(--info);
  }
</style>

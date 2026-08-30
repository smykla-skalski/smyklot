<script lang="ts">
  import Icon from './Icon.svelte';

  const {
    label,
    value,
    failed = false,
  }: {
    label: string;
    value: string;
    /** The clipboard refused, so say where the value can be got by hand. */
    failed?: boolean;
  } = $props();
</script>

<!--
@component
A generated link, shown so it can be read and copied by hand.

Two dialogs hand out a single-use invitation link, and both built this from
scratch: a readonly monospace field, and a line saying so when the clipboard
refused. They had drifted apart in every way but the sentence - one wrapped the
field in `.form-field` and the other in `.link-field`, one reported the failure
through `.link-clipboard` and the other as a form error, and only one of the two
ever confirmed that a copy had worked.

Readonly rather than disabled: the value still has to be selectable, because
"copy it by hand" is exactly what this field is for when the clipboard is gone.

The copy button itself is not here. At both call sites it belongs to the dialog's
footer, beside Done, and dragging it into the field would move it out of the row
of actions a reader is looking at.
-->

<label class="link-field">
  <span>{label}</span>
  <input class="text-input mono" readonly {value} />
</label>
{#if failed}
  <p class="link-clipboard" role="alert">
    <Icon name="alert" size="sm" strokeWidth={2} />
    Copy it from the field above, the clipboard was not available
  </p>
{/if}

<style>
  .link-field {
    display: grid;
    gap: var(--space-2);
  }

  .link-clipboard {
    align-items: center;
    color: var(--warning);
    display: flex;
    font-size: var(--font-size-compact);
    gap: 0.35rem;
    margin: 0.4rem 0 0;
  }
</style>

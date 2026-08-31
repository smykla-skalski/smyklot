<!--
@component
A time somebody can read either way: how long ago by default, and the whole
instant one press away.

Never a `title`. A native tooltip is a hover, so it does not exist on a phone,
cannot be reached from a keyboard, and cannot be copied - and the exact stamp
is the thing a reader wants to paste into a message. Pressing swaps the text
in place instead.

The affordance is a dotted underline on the words, and the hit area is a
pseudo-element at negative inset: a real 24px box would inflate the line and
push apart every pill and row one of these sits in.
-->

<script lang="ts">
  import { formatExact, formatRelative, formatUntil } from '../format';

  const {
    value,
    nowMs = 0,
    future = false,
    exact: exactFirst = false,
    label,
    class: className = '',
  }: {
    /** The instant, as RFC3339. */
    value: string;
    /**
     * The clock to read against, so a page's rows all agree on "now". Read only
     * when the resting text is computed here - a caller passing `label` has
     * already decided what the words are and owes no clock.
     */
    nowMs?: number;
    /** An instant still ahead: "in 4 minutes" rather than "4 minutes ago". */
    future?: boolean;
    /** Start on the whole instant - for a surface whose reader chose that. */
    exact?: boolean;
    /**
     * The resting reading, where a surface has already decided one - a decision
     * date reads as a date. The press still gives the whole instant, which is
     * the point: a stamp nobody can reach is a stamp nobody has.
     */
    label?: string;
    class?: string;
  } = $props();

  const relative = $derived(
    label ?? (future ? formatUntil(value, nowMs) : formatRelative(value, nowMs)),
  );
  const whole = $derived(formatExact(value));

  /* Seeded from the surface's own choice and owned by whoever presses it, and
     seeded again when that choice changes - a table whose reader switches every
     row to absolute means this row too. */
  let showing = $derived(exactFirst);

  function toggle(event: Event): void {
    event.preventDefault();
    event.stopPropagation();
    showing = !showing;
  }

  function keys(event: KeyboardEvent): void {
    if (event.key !== 'Enter' && event.key !== ' ') return;
    toggle(event);
  }
</script>

<!-- A `<time>` and not a `<button>`: the element has to stay a time for its
     `datetime` to mean anything, and a button around it would put a second box
     inside every pill one of these sits in. The whole custom-button recipe is
     here - role, tabindex, Enter and Space, and a label saying both readings. -->
<!-- svelte-ignore a11y_no_noninteractive_element_to_interactive_role -->
<time
  class={className}
  class:is-exact={showing}
  data-exact
  datetime={value}
  role="button"
  tabindex="0"
  aria-label="{relative} - exactly {whole}"
  onclick={toggle}
  onkeydown={keys}>{showing ? whole : relative}</time
>

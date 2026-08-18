<script module lang="ts">
  /**
   * What the button is for, not what colour it is.
   *
   * `default` is the bordered control; `signal` is the one action a view is here
   * for; `ghost` dismisses and discards beside a real primary; `stop` is
   * destructive; `stop-quiet` is destructive but bordered, for a flow whose filled
   * danger control is the confirmation at the end of it; `brand` is the console's
   * own action, tinted with whatever `--brand-action` is where it is drawn; `quiet`
   * is a control that should not read as a control until it is wanted.
   */
  export type ButtonTone =
    'default' | 'signal' | 'ghost' | 'stop' | 'stop-quiet' | 'brand' | 'quiet';
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLAnchorAttributes, HTMLButtonAttributes } from 'svelte/elements';

  // `let` rather than `const` because `element` is bindable, as in the three other
  // components here that hand an element back to a caller.
  let {
    tone = 'default',
    row = false,
    href,
    icon,
    trailing,
    element = $bindable(null),
    class: extra = '',
    children,
    ...rest
  }: {
    tone?: ButtonTone;
    /** Inside a table row, where the control is shorter and tighter than in a header. */
    row?: boolean;
    /** Draws an anchor instead. Sign-in and the invitation's accept and decline are links. */
    href?: string;
    /** Drawn before the label. Its optical bearing is handled by `app.css`, keyed on position. */
    icon?: Snippet;
    /** Drawn after the label - a chevron, or a count. */
    trailing?: Snippet;
    /** For a caller that has to focus this button again after a dialog closes. */
    element?: HTMLButtonElement | null;
    /** The one-off class a call site adds for its own layout, never for the button's own paint. */
    class?: string;
    children: Snippet;
  } & HTMLButtonAttributes &
    HTMLAnchorAttributes = $props();

  const classes = $derived(
    ['btn', tone === 'default' ? '' : `btn-${tone}`, row ? 'btn-row' : '', extra]
      .filter((name) => name !== '')
      .join(' '),
  );
</script>

<!--
  The label is always wrapped. A button is a flex container, so bare text sits in an
  anonymous box no selector can reach and `text-box` trimming on the control never
  touches it; `.button-label` is the only thing that puts the word on its cap height.
  Nine files remembered that and fifteen did not, which is the drift this component
  exists to end. Inside a `.btn` it is the same trim as `.cap-trim`, whose
  `line-height: 1` the button's own font shorthand has already set.
-->
{#snippet body()}
  {#if icon !== undefined}
    {@render icon()}
  {/if}
  <span class="button-label">{@render children()}</span>
  {#if trailing !== undefined}
    {@render trailing()}
  {/if}
{/snippet}

<!--
  No `<style>` block here, and there must never be one.

  `.btn` and its tones live in `app.css` because they are worn by anchors as well as
  buttons - which is the branch below - because call sites add their own classes
  beside them, and because the icon-bearing rule reaches in from outside on
  `:is(button, .btn, a) > svg`. A rule in this file would carry Svelte's scope class
  and outrank all three.

  `type` is written before the spread so a caller can still say `submit`; every other
  attribute a call site passes - `disabled`, `form`, `onclick`, `aria-label` -
  arrives through `rest`.
-->
{#if href !== undefined}
  <a {href} {...rest} class={classes}>{@render body()}</a>
{:else}
  <button bind:this={element} type="button" {...rest} class={classes}>{@render body()}</button>
{/if}

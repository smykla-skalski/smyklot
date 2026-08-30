<script lang="ts">
  import type { ErrorContent } from '../panel-error';
  import Button from './Button.svelte';

  const {
    content,
    panelHref,
    signInHref,
  }: {
    content: ErrorContent;
    panelHref: string;
    signInHref: string;
  } = $props();

  const actionHref = $derived(
    content.action === null ? null : content.action.kind === 'sign-in' ? signInHref : panelHref,
  );
</script>

<!--
@component
What an error looks like inside the card, wherever the card is.

The server's error pages are this on its own, and the invitation page shows it
too when the token names nothing: a link that leads nowhere is a link that
leads nowhere, and telling the reader which of the panel's features the
address would have belonged to only describes something they cannot reach.

Three things, in the order the questions arrive: which error it was, what that
means, and the one thing worth doing about it.
-->

<div class="error-body">
  <!-- Decorative: the sentence under it says the same thing in words, and a
       reader who is being read this page is better served by "this address does
       not lead anywhere" than by the number three digits at a time. -->
  <!-- Trimmed to the digits themselves. At this size the font's leading is worth
       most of a line above the numerals, and a centred block measured with that
       leading in it sits visibly low: the gap a reader sees above the number came
       out larger than the one under the button, because only one of the two had
       empty space built into its box. Measured at 31px against 19px before the
       trim and 28 against 30 after it, the two left being the cap metric rather
       than the layout. The same trim the footer uses. -->
  <p class="error-code band-trim" aria-hidden="true">{content.status}</p>
  <p class="error-lead">{content.lead}</p>
  <p class="error-note">{content.note}</p>
  {#if content.action !== null && actionHref !== null}
    <p class="error-action">
      <Button
        tone="signal"
        href={actionHref}
        rel={content.action.kind === 'sign-in' ? 'nofollow' : undefined}
      >
        {content.action.label}
      </Button>
    </p>
  {/if}
</div>

<style>
  /* The number and the words are one block, centred as one. */
  .error-body {
    display: grid;
    gap: var(--space-2);
    justify-items: center;
    text-align: center;
  }

  /* Neutral, and a step back from the sentence under it.
     ---------------------------------------------------
     The number is the largest thing on the card and the least useful, so it takes
     the quietest ink on it. Colour here is not decoration going spare: the accent
     is what marks the one thing on the page worth clicking, and spending it on a
     five-rem ornament leaves the button arguing with a number eight times its size
     for the same signal. Neutrals carry the display type, the accent carries the
     action - the ordinary reading of the 60-30-10 split, and what the panel does
     everywhere else.

     Size already gives the number all the prominence it needs; taking the value
     back is what stops that prominence reading as importance. */
  .error-code {
    /* One number for how far back the ink stands, so the rule here and the
       keyframe that lands on it cannot drift apart. */
    --error-code-ink: 1;

    color: var(--text-muted);
    font: 800 clamp(3.5rem, 16vw, 5.25rem) / var(--leading-flat) var(--sans);
    letter-spacing: 0.04em;
    margin: 0 0 var(--space-1);
    opacity: var(--error-code-ink);
  }

  /* The sentence the number is a shorthand for. Sized as the card's own lead
     rather than as a heading: the page already has one, above the card. */
  .error-lead {
    color: var(--text-primary);
    font: 650 1.0625rem / var(--leading-body) var(--sans);
    margin: 0;
  }

  /* Why, and what changes it. Measure kept short so the page reads as three short
     lines rather than a paragraph a reader has to work through. */
  .error-note {
    color: var(--text-secondary);
    margin: 0;
    max-width: 30rem;
  }

  .error-action {
    margin: var(--space-3) 0 0;
  }

  /* Once, on arrival, and never again. It is the first thing to read, and this is
     what puts it there rather than having it already waiting. Nothing depends on
     the animation finishing, so under reduced motion it simply does not run. */
  @media (prefers-reduced-motion: no-preference) {
    .error-code {
      animation: error-code-settle var(--duration-normal) var(--ease-standard) both;
    }
  }

  @keyframes error-code-settle {
    from {
      opacity: 0;
      translate: 0 0.35rem;
    }

    to {
      opacity: var(--error-code-ink);
      translate: none;
    }
  }
</style>

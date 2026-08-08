<script lang="ts">
  import { formatCountdown, formatTimestamp } from '../lib/format';
  import { linkPhase } from '../lib/pairing-state';

  const {
    leftMs,
    fractionLeft,
    expiresAt,
  }: {
    /** Milliseconds until the link lapses, or `null` if its deadline is unreadable. */
    leftMs: number | null;
    /** How much of the link's whole lifetime is left, from 0 to 1. */
    fractionLeft: number;
    expiresAt: string;
  } = $props();

  const phase = $derived(linkPhase(leftMs, fractionLeft));
</script>

{#if leftMs !== null}
  <div class="track" aria-hidden="true">
    <div class="fill fill-{phase}" style="width: {fractionLeft * 100}%"></div>
  </div>
{/if}

<p class="meta mono">
  <!-- Ticking text in a live region would be read out every second, so the
       number is shown and the deadline itself is announced instead. -->
  {#if leftMs === null}
    <span>expiry unknown</span>
  {:else if leftMs <= 0}
    <span class="gone">expired</span>
  {:else}
    <!-- Coloured with the gauge, so the number and the bar never disagree about
         how much is left. -->
    <span class="count count-{phase}" aria-hidden="true">
      expires in {formatCountdown(leftMs)}
    </span>
  {/if}
  <!-- Only when the deadline parsed. Otherwise this would put an invalid value
       in `datetime` and announce an expiry the visible label has just said is
       unknown. -->
  {#if leftMs !== null}
    <span class="visually-hidden">
      expires <time datetime={expiresAt}>{formatTimestamp(expiresAt)}</time>
    </span>
  {/if}
</p>

<style>
  /* Draining as the lifetime burns down, because the only thing worth knowing
     about a one-time link is how much of it is left. */
  .track {
    background: var(--rule);
    border-radius: 999px;
    height: 3px;
    margin: 0.75rem 0 0.5rem;
    overflow: hidden;
  }

  .fill {
    height: 100%;
    transition: width 1s linear;
  }

  .fill-ample {
    background: var(--clear);
  }

  .fill-low {
    background: var(--signal);
  }

  .fill-warn {
    background: var(--stop);
  }

  /* A hard on-off rather than a fade: this is the last ten seconds of a
     credential, and a pulse that eases is read as decoration. `steps(1, end)`
     is what makes it switch rather than breathe. */
  .fill-critical {
    animation: blink 0.9s steps(1, end) infinite;
    background: var(--stop);
  }

  .fill-spent {
    background: var(--rule);
  }

  @keyframes blink {
    50% {
      opacity: 0.2;
    }
  }

  /* The colour still says what the flash said, so anyone who asked for stillness
     loses nothing but the flashing. */
  @media (prefers-reduced-motion: reduce) {
    .fill {
      transition: none;
    }

    .fill-critical {
      animation: none;
    }
  }

  .meta {
    align-items: center;
    color: var(--dim);
    display: flex;
    flex-wrap: wrap;
    font-size: 0.6875rem;
    gap: 0.4rem;
    letter-spacing: 0.04em;
    margin: 0;
    text-transform: uppercase;
  }

  /* The one number on the page that changes while it is being read. */
  .count {
    font-size: 0.75rem;
  }

  .count-low {
    color: var(--signal);
  }

  .count-warn,
  .count-critical,
  .gone {
    color: var(--stop);
    font-weight: 600;
  }
</style>

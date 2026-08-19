<script lang="ts">
  /**
   * The bar that says what applying would do, and offers to do it.
   *
   * The sentence is the blast radius, said in the order a person needs it: how
   * many changes, across how many repositories, how many of them take something
   * away, and which of them reach GitHub directly rather than as a pull request.
   * The button names the radius too - "Apply to 3 repositories", never "Apply" -
   * because the button is what gets pressed, and a label that names its scope is
   * the last chance to notice the scope is wrong.
   *
   * The wording lives here rather than at each caller so it stays one sentence
   * that has been thought about once.
   *
   * Sticky rather than fixed: it belongs to the plan it sits under, and it stops
   * where the plan stops instead of floating over whatever comes next.
   */
  import Button from '#lib/components/Button.svelte';

  const {
    changes,
    repositories,
    removals = 0,
    asPullRequests = false,
    applying = false,
    onApply,
  }: {
    changes: number;
    repositories: number;
    /** Counted out loud, in the danger ink: this is what approval is asked for. */
    removals?: number;
    /** Some of this lands as pull requests, which is a different promise. */
    asPullRequests?: boolean;
    applying?: boolean;
    onApply?: () => void;
  } = $props();

  /* Both words, rather than an `s` bolted on: this sentence says "repositories"
     and no rule that stems one word stems that one. */
  const plural = (count: number, one: string, many: string) =>
    `${count} ${count === 1 ? one : many}`;

  const scope = $derived(plural(repositories, 'repository', 'repositories'));
</script>

<div class="plate apply-bar">
  <p class="apply-note band-trim">
    <strong>{plural(changes, 'change', 'changes')} across {scope}</strong>{#if removals > 0},
      including <span class="is-removal">{plural(removals, 'removal', 'removals')}</span>{/if}.
    {#if asPullRequests}
      File changes open pull requests; the rest applies directly.
    {:else}
      Nothing reaches GitHub until you apply.
    {/if}
  </p>
  <!-- One press, because a plan has one answer. There is no discarding a plan:
       leaving it is what not approving it looks like, and the next sweep works
       out a new one from whatever the documents say by then. -->
  <!-- The last press before anything reaches GitHub, so it takes the filled
       tone rather than the bordered one - the same weight the overview's
       "Review the plan" carries at the other end of the same journey. -->
  <Button tone="signal" disabled={applying} onclick={() => onApply?.()}>
    {applying ? 'Applying…' : `Apply to ${scope}`}
  </Button>
</div>

<style>
  /* `.plate` is the card - the ground, the keyline, the radius and the shadow -
     and this had spelled all four out again through the aliases (`--strip` IS
     `--surface-base`, `--rule` IS `--border-subtle`). What is left is what
     makes this plate a BAR: a row, and one that stays with the reader. */
  .apply-bar {
    align-items: center;
    bottom: var(--space-4);
    display: flex;
    gap: var(--space-4);
    margin-block: var(--space-5) 0;
    padding: var(--space-3) var(--space-4);
    position: sticky;
    z-index: var(--layer-sticky);
  }

  .apply-note {
    color: var(--text-secondary);
    flex: 1;
    font-size: var(--font-size-meta);
    margin: 0;
  }

  .apply-note strong {
    color: var(--text-primary);
  }

  .is-removal {
    color: var(--danger);
    font-weight: 600;
  }

  @media (max-width: 40rem) {
    .apply-bar {
      align-items: stretch;
      flex-direction: column;
      gap: var(--space-3);
    }
  }
</style>

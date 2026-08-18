<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Callout, { type CalloutTone } from '#lib/components/Callout.svelte';
  import Icon from '#lib/components/Icon.svelte';

  const TONES: CalloutTone[] = ['quiet', 'warning'];

  const { Story } = defineMeta({
    title: 'Primitives/Callout',
    component: Callout,
    argTypes: { tone: { control: 'select', options: TONES } },
    args: { tone: 'quiet' },
  });
</script>

<Story name="Playground">
  {#snippet template({ children, ...args })}
    <Callout {...args}>
      {#snippet icon()}<Icon name="info" size={20} />{/snippet}
      <span>Review the account and effect before confirming.</span>
    </Callout>
  {/snippet}
</Story>

<!--
  Eight of these were written by hand under three class names, and three of the four
  declarations were the same box. The fourth reached for `--well`, `--rule` and
  `--r-well`, which are aliases of the tokens the others name; only the background
  genuinely differed - `#eeebf4` against `#f0ecf6`, under a just-noticeable
  difference - so unifying them moved one box by less than an eye can resolve.
-->
<Story name="Both tones">
  {#snippet template()}
    <div class="stack">
      <Callout>
        {#snippet icon()}<Icon name="info" size={20} />{/snippet}
        <span>Review the account and effect before confirming.</span>
      </Callout>
      <Callout tone="warning">
        {#snippet icon()}<Icon name="warning" size={18} />{/snippet}
        <span>
          This installation is not yours. Continue to its Access view to acknowledge and start the
          audited 15-minute elevation before adding the user.
        </span>
      </Callout>
    </div>
  {/snippet}
</Story>

<!--
  The two align differently, on purpose. A one-line consequence beside a mark is a
  row, so it centres; a warning runs to several lines, and a mark centred against a
  paragraph floats in the middle of it rather than marking where it starts.
-->
<Story name="Why the marks align differently">
  {#snippet template()}
    <div class="stack">
      <Callout>
        {#snippet icon()}<Icon name="info" size={20} />{/snippet}
        <span>One line, so the mark sits on its centre</span>
      </Callout>
      <Callout tone="warning">
        {#snippet icon()}<Icon name="warning" size={18} />{/snippet}
        <span>
          Several lines, so the mark sits at the top and marks where the warning begins rather than
          floating somewhere in the middle of it. This is the same reason a bullet is not centred
          against its paragraph.
        </span>
      </Callout>
    </div>
  {/snippet}
</Story>

<!-- One call site passes a heading and a paragraph rather than a single line. -->
<Story name="With a heading">
  {#snippet template()}
    <Callout>
      {#snippet icon()}<span class="warning-mark" aria-hidden="true">!</span>{/snippet}
      <div>
        <strong>Declining was an answer</strong>
        <p>
          A new link reaches the same GitHub identity, and asking twice is visible to them and in
          the audit record.
        </p>
      </div>
    </Callout>
  {/snippet}
</Story>

<style>
  .stack {
    display: grid;
    gap: var(--space-4);
    max-width: 34rem;
  }
  .warning-mark {
    align-items: center;
    background: var(--warning-tint);
    border-radius: 50%;
    color: var(--warning);
    display: inline-flex;
    font: 700 0.8rem / 1 var(--sans);
    height: 1.25rem;
    justify-content: center;
    width: 1.25rem;
  }
</style>

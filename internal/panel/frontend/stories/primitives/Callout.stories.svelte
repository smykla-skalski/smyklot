<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Callout, { type CalloutTone } from '#lib/components/Callout.svelte';
  import Icon from '#lib/components/Icon.svelte';
  import Button from '#lib/components/Button.svelte';

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
      {#snippet icon()}<Icon name="info" size="md" />{/snippet}
      <span>Review the account and effect before confirming</span>
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
        {#snippet icon()}<Icon name="info" size="md" />{/snippet}
        <span>Review the account and effect before confirming</span>
      </Callout>
      <Callout tone="warning">
        {#snippet icon()}<Icon name="warning" size="md" />{/snippet}
        <span>
          This workspace is not yours. Continue to its Access view to acknowledge and start the
          audited 15-minute elevation before adding the user
        </span>
      </Callout>
    </div>
  {/snippet}
</Story>

<!-- Every tone uses the first text line, including when a message wraps. -->
<Story name="First line alignment">
  {#snippet template()}
    <div class="stack">
      <Callout>
        {#snippet icon()}<Icon name="info" size="md" />{/snippet}
        <span>One line, with its symbol centered on the text</span>
      </Callout>
      <Callout tone="warning">
        {#snippet icon()}<Icon name="warning" size="md" />{/snippet}
        <span>
          A wrapped message keeps its symbol centered on the first line, so the same alignment works
          at every width
        </span>
      </Callout>
    </div>
  {/snippet}
</Story>

<!-- One call site passes a heading and a paragraph rather than a single line. -->
<Story name="With a heading">
  {#snippet template()}
    <Callout>
      {#snippet icon()}<Icon name="warning" size="md" />{/snippet}
      <div class="callout-copy">
        <strong>Declining was an answer</strong>
        <p>
          A new link reaches the same GitHub identity, and asking twice is visible to them and in
          the audit record
        </p>
      </div>
    </Callout>
  {/snippet}
</Story>

<Story name="With an action">
  {#snippet template()}
    <Callout tone="warning">
      {#snippet icon()}<Icon name="warning" size="base" />{/snippet}
      <div class="callout-copy">
        <strong>Changes need attention</strong>
        <p>Review the workspace before applying your saved draft</p>
      </div>
      {#snippet actions()}<Button tone="quiet">Review</Button>{/snippet}
    </Callout>
  {/snippet}
</Story>

<style>
  .stack {
    display: grid;
    gap: var(--space-4);
    max-width: 34rem;
  }
</style>

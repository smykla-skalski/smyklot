<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import IdentityRow from '#lib/components/IdentityRow.svelte';
  import Avatar from '#lib/components/Avatar.svelte';
  import type { PanelAccount } from '#lib/types.js';

  const ACCOUNT: PanelAccount = {
    id: '1001',
    provider: 'github:https://api.github.com',
    subject_id: '1001',
    login: 'ada',
    display_name: 'Ada Lovelace',
    avatar_url: null,
  };

  const { Story } = defineMeta({ title: 'Primitives/IdentityRow', component: IdentityRow });
</script>

<!--
  The case the component exists for: a mark beside a two-line stack, where the
  *words* have to sit on the mark's centre rather than the box that holds them.

  `.band-trim-stack` gives back the leading above the first line's capitals and the
  room under the last line's baseline, so the stack IS the band being centred. Two of
  the five call sites had that and three did not; one of those three measured 0.000px
  off anyway, because at its size the two happened to cancel - correct by luck, and
  only for that pairing of fonts.
-->
<Story name="Default">
  {#snippet template()}
    <IdentityRow>
      {#snippet mark()}<Avatar account={ACCOUNT} size={32} />{/snippet}
      {#snippet name()}<strong>{ACCOUNT.display_name}</strong>{/snippet}
      {#snippet handle()}<span class="mono">@{ACCOUNT.login}</span>{/snippet}
    </IdentityRow>
  {/snippet}
</Story>

<!--
  Descenders are the other half. The trim ends the last line's box on its baseline,
  but it moves the box and not the glyphs - so the tail of a `y`, a `g` or an `@`
  still paints below it. `overflow: clip` with a 0.4em margin lets that ink through
  without letting it reach the row underneath; `hidden` sheared it off.

  Chrome is the only engine that implements the trim, so it was the only one showing
  the damage.
-->
<Story name="Descenders">
  {#snippet template()}
    <IdentityRow>
      {#snippet mark()}
        <Avatar
          account={{ ...ACCOUNT, login: 'gypsy', display_name: 'Peggy Guggenheim' }}
          size={32}
        />
      {/snippet}
      {#snippet name()}<strong>Peggy Guggenheim</strong>{/snippet}
      {#snippet handle()}<span class="mono">@peggy-guggenheim-jr</span>{/snippet}
    </IdentityRow>
  {/snippet}
</Story>

<!-- A name that will not fit truncates; the row stays one line tall either way. -->
<Story name="Truncated">
  {#snippet template()}
    <div class="narrow">
      <IdentityRow>
        {#snippet mark()}<Avatar account={ACCOUNT} size={32} />{/snippet}
        {#snippet name()}<strong>A display name far too long for the column it is in</strong
          >{/snippet}
        {#snippet handle()}<span class="mono">@a-login-that-also-runs-past-the-edge</span>{/snippet}
      </IdentityRow>
    </div>
  {/snippet}
</Story>

<!-- The installations table leads with a monogram and links the name. -->
<Story name="Monogram and link">
  {#snippet template()}
    <IdentityRow>
      {#snippet mark()}<span class="monogram"><span class="cap-trim">SS</span></span>{/snippet}
      {#snippet name()}<a class="link" href="#/">Smykla Skalski</a>{/snippet}
      {#snippet handle()}<span class="mono">@smykla-skalski · #3001</span>{/snippet}
    </IdentityRow>
  {/snippet}
</Story>

<!-- Several rows, to check the marks and the stacks line up down the column. -->
<Story name="A column of them">
  {#snippet template()}
    <div class="rows">
      {#each ['Ada Lovelace', 'Grace Hopper', 'Katherine Johnson'] as person (person)}
        <IdentityRow>
          {#snippet mark()}
            <Avatar account={{ ...ACCOUNT, display_name: person }} size={32} />
          {/snippet}
          {#snippet name()}<strong>{person}</strong>{/snippet}
          {#snippet handle()}
            <span class="mono">@{person.split(' ')[0]?.toLowerCase()}</span>
          {/snippet}
        </IdentityRow>
      {/each}
    </div>
  {/snippet}
</Story>

<style>
  .narrow {
    max-width: 18rem;
  }
  .rows {
    display: grid;
    gap: var(--space-4);
  }
  .monogram {
    align-items: center;
    background: var(--surface-inset);
    border-radius: var(--radius-control);
    display: inline-flex;
    font: 700 var(--font-size-compact) / 1 var(--sans);
    height: 2rem;
    justify-content: center;
    width: 2rem;
  }
</style>

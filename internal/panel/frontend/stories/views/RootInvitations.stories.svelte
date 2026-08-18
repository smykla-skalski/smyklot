<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import RootInvitations from '#lib/components/RootInvitations.svelte';
  import type { PanelInvitation } from '#lib/types.js';
  import Seeded from '../support/Seeded.svelte';
  import { INVITATIONS } from '../support/fixtures.js';

  /**
   * The key the view asks for on its first render, and it has to match exactly.
   *
   * `createInfiniteQuery` keys on every input to the request - the search, the sort,
   * the selected statuses and the page size - so a seed that guesses any of them
   * lands under a different key, the view finds nothing, and the story shows the
   * loading state forever. These are the component's own initial values.
   */
  const KEY = ['root-access', 'invitations', '', 'created_newest', [], 20] as const;

  /** An infinite query's cache entry is pages, not rows. */
  const page = (items: PanelInvitation[]) => ({
    pages: [{ items, next_cursor: null, total: items.length }],
    pageParams: [undefined],
  });

  const base = {
    fetchPage: () => Promise.resolve({ items: INVITATIONS, next_cursor: null, total: 3 }),
    create: fn(),
    reissue: fn(),
    revoke: fn(),
    canManage: true,
    actorLogin: 'bart',
  };

  const { Story } = defineMeta({
    title: 'Views/RootInvitations',
    component: RootInvitations,
    args: base,
  });
</script>

<!--
  Who has been asked to hold Root, and how far each of them has got with it. Every
  row is one of the four states an invitation can be in; the table is the same one
  the access list draws, which is why both moved onto `DataTable` together.
-->
<Story name="Invitations">
  {#snippet template(args)}
    <Seeded seed={[[KEY, page(INVITATIONS)]]}><RootInvitations {...args} /></Seeded>
  {/snippet}
</Story>

<!--
  Nobody invited. Not a fault and not a wait - the empty row says so in words rather
  than leaving a header standing over nothing.
-->
<Story name="Empty">
  {#snippet template(args)}
    <Seeded seed={[[KEY, page([])]]}>
      <RootInvitations
        {...args}
        fetchPage={() => Promise.resolve({ items: [], next_cursor: null, total: 0 })}
      />
    </Seeded>
  {/snippet}
</Story>

<!--
  A reader who may not act. The toolbar keeps its search and its filters, because
  reading is still reading; what goes is every control that would change something.
-->
<Story name="Read only">
  {#snippet template(args)}
    <Seeded seed={[[KEY, page(INVITATIONS)]]}>
      <RootInvitations {...args} canManage={false} />
    </Seeded>
  {/snippet}
</Story>

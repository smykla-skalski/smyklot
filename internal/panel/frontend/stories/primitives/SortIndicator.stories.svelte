<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import SortIndicator from '#lib/components/SortIndicator.svelte';

  const { Story } = defineMeta({ title: 'Primitives/SortIndicator', component: SortIndicator });
</script>

<!--
  The one sort arrow. Four tables kept their own, one of them written against the raw
  `<svg>`. It is a single chevron, trailing - the pair was tried and rejected - and it
  does not move while the pointer is on it.

  Its direction and visibility come from the heading around it, keyed on `aria-sort`,
  so there is nothing to pass in here.
-->
<Story name="Default">
  {#snippet template()}
    <table>
      <thead>
        <tr>
          <th scope="col" aria-sort="ascending">
            <button class="table-sort-button" type="button">
              <span class="table-heading-label">Ascending</span><SortIndicator />
            </button>
          </th>
          <th scope="col" aria-sort="descending">
            <button class="table-sort-button" type="button">
              <span class="table-heading-label">Descending</span><SortIndicator />
            </button>
          </th>
          <th scope="col" aria-sort="none">
            <button class="table-sort-button" type="button">
              <span class="table-heading-label">Unsorted</span><SortIndicator />
            </button>
          </th>
        </tr>
      </thead>
    </table>
  {/snippet}
</Story>

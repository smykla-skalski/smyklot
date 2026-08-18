<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import DataTable from '#lib/components/DataTable.svelte';
  import Chip from '#lib/components/Chip.svelte';
  import IdentityRow from '#lib/components/IdentityRow.svelte';
  import TableEmptyState from '#lib/components/TableEmptyState.svelte';
  import { INSTALLATIONS } from '../support/fixtures.js';

  type Installation = (typeof INSTALLATIONS)[number];

  const COLUMNS = [{ label: 'Installation' }, { label: 'Repositories' }, { label: 'Ownership' }];

  const { Story } = defineMeta({
    title: 'Primitives/DataTable',
    component: DataTable,
    /* `rows` and `rowKey` stay out of `args`: `DataTable` is generic in its row, and
       Storybook's meta types the component as `unknown`, so the generic only infers
       where the props are written on the element itself. */
    args: {
      caption: 'Installation catalog',
      regionLabel: 'Installation catalog table',
      columns: COLUMNS,
    },
  });
</script>

<!--
  The shell nine tables wrote by hand under six wrapper class names.

  The `<tr>` is rendered here and the caller supplies only the cells. That is not a
  style choice: a `<tr>` rendered by a child carries the child's scope class, so every
  `tbody tr` rule in the parent stops matching - measured once at headings sitting 3.4k
  pixels from their cells.
-->
<!--
  These stories do not spread `args`, and that is deliberate rather than an oversight.
  `DataTable` is generic in `Row`, and Storybook types a meta's args as the component's
  props with the generic left at `unknown` - so spreading them pins `Row` to `unknown`
  and every `rowKey` and `cells` written against a real type becomes an error. Each
  story states its own props instead, which is also what makes them readable as
  examples.
-->

<Story name="Rows">
  {#snippet template()}
    <DataTable
      caption="Installation catalog"
      regionLabel="Installation catalog table"
      columns={COLUMNS}
      rows={INSTALLATIONS}
      rowKey={(row: Installation) => row.id}
    >
      {#snippet cells(installation: Installation)}
        <th scope="row">
          <IdentityRow>
            {#snippet mark()}<span class="mono">{installation.account.login.slice(0, 2)}</span
              >{/snippet}
            {#snippet name()}<strong>{installation.account.display_name}</strong>{/snippet}
            {#snippet handle()}<span class="mono">@{installation.account.login}</span>{/snippet}
          </IdentityRow>
        </th>
        <td>{installation.repository_counts.enabled} of {installation.repository_counts.total}</td>
        <td>
          <Chip tone={installation.ownership.stale ? 'neutral' : 'clear'} dot>
            {installation.ownership.stale ? 'Stale' : 'Fresh'}
          </Chip>
        </td>
      {/snippet}
    </DataTable>
  {/snippet}
</Story>

<!--
  The empty row and its spanning cell are the shell's, not the caller's: nine tables
  wrote that pair by hand, and the `colspan` has to agree with the column count or the
  cell stops spanning.
-->
<Story name="Empty">
  {#snippet template()}
    <DataTable
      caption="Installation catalog"
      regionLabel="Installation catalog table"
      columns={COLUMNS}
      rows={[]}
      rowKey={(row: Installation) => row.id}
    >
      {#snippet cells()}{/snippet}
      {#snippet empty()}
        <TableEmptyState
          title="No installations match"
          description="Nothing in the catalog matches this search"
          actionLabel="Clear search"
          onAction={fn()}
        />
      {/snippet}
    </DataTable>
  {/snippet}
</Story>

<!-- Rows that open something take their attributes from the caller, not their markup. -->
<Story name="Clickable rows">
  {#snippet template()}
    <DataTable
      caption="Installation catalog"
      regionLabel="Installation catalog table"
      columns={COLUMNS}
      rows={INSTALLATIONS}
      rowKey={(row: Installation) => row.id}
      rowAttrs={() => ({ class: 'data-row', tabindex: 0, onclick: fn() })}
    >
      {#snippet cells(installation: Installation)}
        <th scope="row">{installation.account.display_name}</th>
        <td>{installation.repository_counts.total}</td>
        <td><Chip tone="clear" dot>Fresh</Chip></td>
      {/snippet}
    </DataTable>
  {/snippet}
</Story>

<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import PolicyGroup from '#lib/components/PolicyGroup.svelte';
  import PolicyRow from '#lib/components/PolicyRow.svelte';
  import Switch from '#lib/components/Switch.svelte';

  const { Story } = defineMeta({
    title: 'Views/PolicyGroup',
    component: PolicyGroup,
  });

  /** One set, said as a sentence while the picker is shut and as chips once it opens. */
  const FEATURES = [
    { key: 'has_issues', label: 'Issues' },
    { key: 'has_projects', label: 'Projects' },
    { key: 'has_wiki', label: 'Wiki' },
    { key: 'has_discussions', label: 'Discussions' },
    { key: 'has_sponsorships', label: 'Sponsorships' },
  ];
</script>

<script lang="ts">
  // Live, because the point of the row is that the word and the control say
  // the same thing at the same moment.
  let squash = $state(true);
  let merge = $state(false);
  let auto = $state(true);
  let deleteBranch = $state(true);
  let scanning = $state(true);
</script>

<!--
  The page is the policy: what this installation decides is a row, and the six
  settings it leaves alone are one sentence that names them. The value is said
  as a word beside its control, so a column of these is read rather than
  decoded, and the clear at the end of a row stops the managing - it never
  writes the opposite value.
-->
<Story name="A managed group">
  {#snippet template()}
    <PolicyGroup
      name="Merging"
      managed={4}
      total={6}
      unmanaged={[
        { key: 'allow_rebase_merge', label: 'Rebase merging' },
        { key: 'allow_update_branch', label: 'Offer to update the branch' },
      ]}
      onManage={() => {}}
    >
      <PolicyRow
        name="Squash merging"
        why="A pull request may land squashed to one commit"
        value={squash ? 'On' : 'Off'}
        onStopManaging={() => {}}
      >
        {#snippet control()}
          <Switch
            checked={squash}
            ariaLabel="Squash merging"
            onChange={(next) => {
              squash = next;
            }}
          />
        {/snippet}
      </PolicyRow>

      <PolicyRow
        name="Merge commits"
        why="A pull request may land as a merge commit"
        value={merge ? 'On' : 'Off'}
        onStopManaging={() => {}}
      >
        {#snippet control()}
          <Switch
            checked={merge}
            ariaLabel="Merge commits"
            onChange={(next) => {
              merge = next;
            }}
          />
        {/snippet}
      </PolicyRow>

      <PolicyRow
        name="Auto-merge"
        why="A pull request may be set to merge itself once its checks pass"
        value={auto ? 'On' : 'Off'}
        onStopManaging={() => {}}
      >
        {#snippet control()}
          <Switch
            checked={auto}
            ariaLabel="Auto-merge"
            onChange={(next) => {
              auto = next;
            }}
          />
        {/snippet}
      </PolicyRow>

      <PolicyRow
        name="Delete the branch on merge"
        why="The head branch goes as soon as its pull request lands"
        value={deleteBranch ? 'On' : 'Off'}
        onStopManaging={() => {}}
      >
        {#snippet control()}
          <Switch
            checked={deleteBranch}
            ariaLabel="Delete the branch on merge"
            onChange={(next) => {
              deleteBranch = next;
            }}
          />
        {/snippet}
      </PolicyRow>
    </PolicyGroup>
  {/snippet}
</Story>

<!-- Nothing left to add, so nothing offers to add it. -->
<Story name="Everything in the group is managed">
  {#snippet template()}
    <PolicyGroup name="Security" managed={1} total={1}>
      <PolicyRow
        name="Secret scanning"
        why="GitHub looks for credentials in what is pushed"
        value={scanning ? 'On' : 'Off'}
        onStopManaging={() => {}}
      >
        {#snippet control()}
          <Switch
            checked={scanning}
            ariaLabel="Secret scanning"
            onChange={(next) => {
              scanning = next;
            }}
          />
        {/snippet}
      </PolicyRow>
    </PolicyGroup>
  {/snippet}
</Story>

<!--
  A group this installation has not touched. The rows are empty and the whole
  card is the sentence, which is the honest shape of "nothing is managed here".
-->
<Story name="Nothing managed yet">
  {#snippet template()}
    <PolicyGroup name="Features" managed={0} total={5} unmanaged={FEATURES} onManage={() => {}} />
  {/snippet}
</Story>

<!--
  The picker, open where the Manage press was. The sentence stops naming the
  rest once the chips do - naming the same set twice in one row is the reader
  reading it twice to check they agree.
-->
<Story name="Picking one to manage">
  {#snippet template()}
    <PolicyGroup
      name="Features"
      managed={0}
      total={5}
      unmanaged={FEATURES}
      picking
      onPick={() => {}}
      onCancel={() => {}}
    />
  {/snippet}
</Story>

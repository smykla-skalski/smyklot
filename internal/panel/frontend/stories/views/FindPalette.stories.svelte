<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import FindPalette, { type FindEntry } from '#lib/components/FindPalette.svelte';

  const page = (title: string, say: string, cross?: string): FindEntry => ({
    group: 'Pages',
    title,
    say,
    href: '#',
    cross,
    select: fn(),
  });

  const ENTRIES: FindEntry[] = [
    page('Repositories', 'every repository and its switch'),
    page('Queue', 'what Smyklot is about to do'),
    page('Sync status', 'the sync board - which repositories are settled'),
    page('Rulesets', 'branch protections the sync holds in step'),
    page('Shared files', 'shared files the sync copies around'),
    page('Workspace settings', 'what every repository here inherits'),
    {
      group: 'Workspaces',
      title: 'Acme Robotics',
      say: '@acme-robotics',
      href: '#',
      select: fn(),
    },
    page('Workspaces', 'every workspace the service serves', 'Operations'),
    page('Service health', 'the service and the database it runs on', 'Operations'),
  ];

  const REPOSITORIES: FindEntry[] = [
    {
      group: 'Repositories',
      title: 'api-gateway',
      say: 'on',
      href: '#',
      select: fn(),
    },
    {
      group: 'Repositories',
      title: 'gateway-tests',
      say: 'off - Smyklot stands down there',
      href: '#',
      select: fn(),
    },
  ];

  const { Story } = defineMeta({
    title: 'Views/FindPalette',
    component: FindPalette,
    args: {
      open: true,
      placeholder: 'Search this workspace',
      entries: ENTRIES,
      crossLabel: 'the console',
    },
  });
</script>

<!--
  The palette at rest: nothing typed, and nothing searched before, so it says what
  it can reach rather than showing an empty list.
-->
<Story name="Opened">
  {#snippet template(args)}
    <div class="stage"><FindPalette {...args} /></div>
  {/snippet}
</Story>

<!--
  What is asked for: repositories arrive from the service a moment after the words
  do, so a palette that claimed nothing matched would be wrong every time.
-->
<Story
  name="Asking the service"
  args={{
    lookup: () =>
      new Promise<FindEntry[]>((resolve) => setTimeout(() => resolve(REPOSITORIES), 400)),
  }}
>
  {#snippet template(args)}
    <div class="stage"><FindPalette {...args} /></div>
  {/snippet}
</Story>

<style>
  .stage {
    block-size: 34rem;
    position: relative;
  }
</style>

<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';
  import BypassActorEditor from '#lib/components/BypassActorEditor.svelte';

  const { Story } = defineMeta({
    title: 'Views/BypassActorEditor',
    component: BypassActorEditor,
    args: {
      actors: [
        { actor_id: 5, actor_type: 'RepositoryRole', bypass_mode: 'always' },
        { actor_id: 1197525, actor_type: 'Integration', bypass_mode: 'always' },
      ],
      lookup: async () => ({
        items: [
          {
            actor_id: 1197525,
            actor_type: 'Integration',
            name: 'smyklot',
            slug: 'smyklot',
            avatar_url: 'https://avatars.githubusercontent.com/in/1197525?v=4',
          },
        ],
      }),
      onChange: fn(),
    },
  });
</script>

<Story name="Release exceptions" />
<Story name="Empty" args={{ actors: [] }} />
<Story name="Inherited" args={{ readOnly: true }} />
<Story
  name="Names unavailable"
  args={{
    lookup: async () => ({
      items: [],
      warning: 'GitHub could not load actor names. Saved exceptions remain available.',
    }),
  }}
/>

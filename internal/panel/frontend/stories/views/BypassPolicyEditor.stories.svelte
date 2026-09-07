<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';
  import BypassPolicyEditor from '#lib/components/BypassPolicyEditor.svelte';

  const allowed = {
    allow: true,
    actors: [{ actor_id: 1197525, actor_type: 'Integration', bypass_mode: 'always' }],
  };
  const { Story } = defineMeta({
    title: 'Views/BypassPolicyEditor',
    component: BypassPolicyEditor,
    args: {
      value: null,
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

<Story name="Keep GitHub exceptions" />
<Story name="Selected actors" args={{ value: allowed }} />
<Story name="No exceptions" args={{ value: { ...allowed, allow: false } }} />
<Story name="Follow workspace" args={{ inherited: allowed }} />
<Story name="Empty actor list" args={{ value: { allow: true, actors: [] } }} />
<Story name="Read only" args={{ value: allowed, readOnly: true }} />

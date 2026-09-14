<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import InstallationPrompt from '#lib/components/InstallationPrompt.svelte';
  import { stubApi } from '../support/api.js';
  const { Story } = defineMeta({
    title: 'Views/InstallationPrompt',
    component: InstallationPrompt,
    args: {
      api: stubApi({
        fetchInstallation: async () => 'https://github.com/apps/smyklot/installations/new',
      }),
      reload: async () => {},
    },
  });
</script>

<Story name="First workspace" />

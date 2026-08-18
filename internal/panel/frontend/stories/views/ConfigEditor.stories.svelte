<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import ConfigEditor from '#lib/components/ConfigEditor.svelte';
  import { CONFIG } from '../support/fixtures.js';

  const { Story } = defineMeta({
    title: 'Views/ConfigEditor',
    component: ConfigEditor,
    argTypes: {
      scope: { control: 'inline-radio', options: ['target', 'repository', 'runtime'] },
      section: { control: 'inline-radio', options: ['all', 'behavior', 'commands'] },
      disabled: { control: 'boolean' },
    },
    args: {
      patch: {},
      inherited: CONFIG,
      scope: 'target',
      idPrefix: 'story',
      section: 'all',
      disabled: false,
      onSave: async () => {},
    },
  });
</script>

<!--
  Every setting the panel can change, and where each value comes from. A row that has
  not been answered here shows what it inherits and says which layer answered it -
  the precedence chain runs deployment, then account, then repository file, then the
  panel's own repository setting.
-->
<Story name="Nothing overridden" />

<!--
  Once a key is answered here the save bar appears and counts what is unsaved. The
  marker beside a changed row is the same one in all four places it is drawn.
-->
<Story
  name="With unsaved changes"
  args={{ patch: { quiet_success: true, allow_self_approval: true } }}
/>

<Story name="Behaviour only" args={{ section: 'behavior' }} />

<Story name="Commands only" args={{ section: 'commands' }} />

<!-- A reader who may look and not touch. -->
<Story name="Disabled" args={{ disabled: true }} />

<!-- The deployment layer, where there is nothing above it to inherit from. -->
<Story name="Runtime scope" args={{ scope: 'runtime', idPrefix: 'story-runtime' }} />

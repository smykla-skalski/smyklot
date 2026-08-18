<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SyncRulesetsForm from '#lib/components/SyncRulesetsForm.svelte';

  const STORED = {
    rulesets: [
      {
        name: 'main',
        target: 'branch',
        enforcement: 'active',
        conditions: { ref_name: { include: ['~DEFAULT_BRANCH'], exclude: [] } },
        rules: [{ type: 'pull_request' }, { type: 'deletion' }],
        bypass_actors: [{ actor_id: 1, actor_type: 'OrganizationAdmin', bypass_mode: 'always' }],
      },
    ],
  };

  const { Story } = defineMeta({
    title: 'Views/SyncRulesetsForm',
    component: SyncRulesetsForm,
    argTypes: {
      enabled: { control: 'boolean' },
      unreadable: { control: 'boolean' },
      readOnly: { control: 'boolean' },
      saving: { control: 'boolean' },
    },
    args: {
      stored: STORED,
      enabled: true,
      unreadable: false,
      readOnly: false,
      saving: false,
      onSave: fn(),
    },
  });
</script>

<!--
  The branch rulesets every repository in an installation should carry. A ruleset is
  several things at once - what it applies to, what it requires, who may bypass it -
  so each block is set off with a heading of its own rather than running together.
-->
<Story name="One ruleset" />

<!-- Nothing configured yet: the form has to offer a way in rather than show a void. -->
<Story name="Empty" args={{ stored: { rulesets: [] } }} />

<Story name="Switched off" args={{ enabled: false }} />

<Story name="Saving" args={{ saving: true }} />

<Story name="Unreadable" args={{ unreadable: true }} />

<Story name="Read only" args={{ readOnly: true }} />

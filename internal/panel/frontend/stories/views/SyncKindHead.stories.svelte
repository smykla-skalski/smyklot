<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SyncKindHead from '#lib/components/SyncKindHead.svelte';

  const { Story } = defineMeta({
    title: 'Views/SyncKindHead',
    component: SyncKindHead,
    args: {
      title: 'Repository settings',
      lead: 'Manage a setting and every repository is held to its value. Anything unmanaged is left exactly as each repository has it, which is not the same as setting it off',
      noun: 'settings',
      enabled: true,
      unreadable: false,
      readOnly: false,
      saving: false,
      onToggle: fn(),
    },
  });
</script>

<!--
  Every kind of sync opens the same way, which is the point of the component:
  its name, what it holds, and the one control that decides whether it runs.
  The switch is a switch because flipping it IS the act - it makes the kind
  eligible for planning and nothing more.
-->
<Story name="Syncing" />

<Story name="Switched off" args={{ enabled: false }} />

<!-- The last save came back refused, and the reason belongs beside this kind. -->
<Story name="With a problem" args={{ problem: 'the settings changed while you were editing' }} />

<!--
  A document written by a newer version. Nothing is shown, because an empty form
  and an unreadable one look identical - and saving from one would wipe the
  other.
-->
<Story name="Unreadable" args={{ unreadable: true }} />

<!--
  Configured but not permitted. Said only while the switch is on: a kind nobody
  asked for is not waiting on anything.
-->
<Story
  name="A permission is missing"
  args={{
    unavailable: 'Smyklot has not been granted administration access, which settings sync needs',
  }}
/>

<Story name="Read only" args={{ readOnly: true }} />

<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import LoginField from '#lib/components/LoginField.svelte';
  import type { PanelAccount } from '#lib/types.js';

  const ROSTER: PanelAccount[] = ['marta-w', 'marek', 'kasia', 'tomasz'].map((login, index) => ({
    id: `roster-${index}`,
    provider: 'github:https://api.github.com',
    subject_id: `${9000 + index}`,
    login,
    display_name: login.replace('-', ' '),
    avatar_url: null,
  }));

  const suggest = async (query: string): Promise<PanelAccount[]> =>
    query === '' ? [] : ROSTER.filter((account) => account.login.includes(query));

  const { Story } = defineMeta({
    title: 'Primitives/LoginField',
    component: LoginField,
    argTypes: { refused: { control: 'boolean' }, focusOnOpen: { control: 'boolean' } },
    args: {
      id: 'story-login',
      value: '',
      label: 'GitHub login',
      suggest,
      refused: false,
      focusOnOpen: false,
    },
  });
</script>

<!--
  Type `mar` to see the suggestions. They portal to `.app-shell`, so they inherit the
  active palette.

  `focusOnOpen` takes focus once, when the dialog holding the field opens. It used to
  set an attribute that nothing read, so the two dialogs asking for it opened with the
  field unfocused.
-->
<Story name="Empty" />

<Story name="With help" args={{ help: 'The GitHub account that will be invited' }} />

<!-- The server refused this login, and the reason sits under the field that caused it. -->
<Story
  name="Refused"
  args={{ value: 'bart', refused: true, help: 'You cannot change your own access' }}
/>

<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SyncLabelsForm from '#lib/components/SyncLabelsForm.svelte';

  const LABELS = [
    { name: 'kind/bug', color: 'd73a4a', description: "Something isn't working" },
    { name: 'kind/enhancement', color: 'a2eeef', description: 'New feature or request' },
    { name: 'area/ci', color: 'fbca04', description: 'CI/CD and automation' },
    // No description at all, which is a different request from an empty one:
    // this leaves whatever each repository wrote where an empty one clears it.
    { name: 'good first issue', color: '7057ff' },
    // White, which is why the swatch is a ring rather than a filled square.
    { name: 'triage/wontfix', color: 'ffffff', description: 'This will not be worked on' },
  ];

  const { Story } = defineMeta({
    title: 'Views/SyncLabelsForm',
    component: SyncLabelsForm,
    argTypes: {
      enabled: { control: 'boolean' },
      unreadable: { control: 'boolean' },
      readOnly: { control: 'boolean' },
      saving: { control: 'boolean' },
    },
    args: {
      labels: LABELS,
      allowRemoval: false,
      excludes: [],
      enabled: true,
      unreadable: false,
      unavailable: '',
      problem: null,
      readOnly: false,
      saving: false,
      onSave: fn(),
    },
  });
</script>

<!--
  The labels every repository in an installation should carry. A card per label,
  because a label is three fields and a row of three inputs stops being readable
  at the width a phone gives it.
-->
<Story name="Configured" />

<!-- Nothing configured yet: the form has to offer a way in rather than show a void. -->
<Story name="Empty" args={{ labels: [] }} />

<!--
  Removal on, with the exclusions beside it. The switch proposes deleting every
  label a repository has that this list does not name, so the list of names to
  spare has to be on the same page as the switch that needs it.
-->
<Story name="Removal on" args={{ allowRemoval: true, excludes: ['hand-made-*', 'dependencies'] }} />

<!--
  A colour that is not six hexadecimal digits yet. The swatch goes blank rather
  than keeping the last value that parsed, which would read as the field having
  been accepted.
-->
<Story
  name="A colour part typed"
  args={{ labels: [{ name: 'kind/bug', color: '#d73a', description: 'Half a colour' }] }}
/>

<Story name="Switched off" args={{ enabled: false }} />

<Story name="Saving" args={{ saving: true }} />

<!--
  Stored in a shape this build cannot read. The list is empty because nothing came
  out of it, not because nothing is configured, so saving over it would wipe a set
  nobody was shown.
-->
<Story name="Unreadable" args={{ unreadable: true, labels: [] }} />

<Story name="Read only" args={{ readOnly: true }} />

<!--
  The server refused the save, and the message belongs beside the form that sent it.
  Named the way the settings and document forms name the same case; "Refused" is the
  files form's name for something else, a planner refusal rather than a failed save.
-->
<Story
  name="With a problem"
  args={{
    problem: 'the color of "kind/bug" must not start with #, GitHub wants "d73a4a"',
  }}
/>

<!-- The installation has not granted what label sync needs. -->
<Story
  name="Unavailable"
  args={{ unavailable: 'Smyklot has not been granted issues access, which labels sync needs' }}
/>

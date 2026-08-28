<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import FormattingEditor from '#lib/components/FormattingEditor.svelte';
  import { defaultFormattingPolicy } from '#lib/formatting.js';

  const { Story } = defineMeta({
    title: 'Views/FormattingEditor',
    component: FormattingEditor,
    argTypes: {
      scope: {
        control: 'inline-radio',
        options: ['target', 'repository', 'runtime', 'template', 'path'],
      },
      disabled: { control: 'boolean' },
    },
    args: {
      patch: {},
      inherited: defaultFormattingPolicy(),
      scope: 'target',
      idPrefix: 'story',
      disabled: false,
      onChange: () => {},
    },
  });
</script>

<!-- Every leaf inherits and the generated contract supplies all groups, choices and bounds. -->
<Story name="Preserve first" />

<!-- A preset reset and an explicit sibling cancellation remain visibly distinct. -->
<Story
  name="Conventional with exceptions"
  args={{
    patch: {
      preset: 'conventional',
      common: { line_width: 120 },
      json: { arrays: 'preserve' },
      markdown: { prose_wrap: 'never' },
    },
    dirtyKeys: [
      'formatting.preset',
      'formatting.common.line_width',
      'formatting.json.arrays',
      'formatting.markdown.prose_wrap',
    ],
  }}
/>

<Story name="Repository scope" args={{ scope: 'repository', idPrefix: 'story-repository' }} />

<Story name="Read only" args={{ disabled: true }} />

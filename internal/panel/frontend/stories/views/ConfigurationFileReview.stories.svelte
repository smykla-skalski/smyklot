<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ConfigurationFileReviewStory from '../support/ConfigurationFileReviewStory.svelte';

  const { Story } = defineMeta({
    title: 'Views/ConfigurationFileReview',
    component: ConfigurationFileReviewStory,
    args: { variant: 'conflict' },
  });
</script>

<Story name="Overlapping edits" />
<Story name="Deleted file" args={{ variant: 'removed' }} />
<Story name="Independent edits" args={{ variant: 'independent' }} />
<Story name="Initial missing file" args={{ variant: 'missing' }} />
<Story name="Invalid TOML" args={{ variant: 'invalid' }} />
<Story name="Unsupported schema" args={{ variant: 'schema' }} />
<Story name="Wrong scope" args={{ variant: 'scope' }} />
<Story name="Missing workspace repository" args={{ variant: 'missingAccess' }} />
<Story name="Outstanding proposal" args={{ variant: 'outstanding' }} />
<Story name="Read only" args={{ readOnly: true }} />
<Story name="Local draft" args={{ hasDrafts: true }} />

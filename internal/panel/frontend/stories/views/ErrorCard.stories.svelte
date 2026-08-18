<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import ErrorCard from '#lib/components/ErrorCard.svelte';
  import { describeFailure } from '#lib/panel-error.js';

  const content = (status: number, code: string) => describeFailure({ status, code, message: '' });

  const { Story } = defineMeta({
    title: 'Views/ErrorCard',
    component: ErrorCard,
    args: {
      content: content(404, 'not_found'),
      panelHref: '#/',
      signInHref: 'https://github.com/login/oauth/authorize',
    },
  });
</script>

<!--
  Three things, in the order the questions arrive: which error it was, what that
  means, and the one thing worth doing about it. At most one action, and only where
  pressing it can actually help - a 404 is terminal, so it offers the panel rather
  than a retry.

  The 404 names nothing: a reader holding a link that leads nowhere cannot act on
  being told which feature the address would have belonged to.
-->
<Story name="Not found" />

<Story name="Sign-in stopped" args={{ content: content(401, 'sign_in_failed') }} />

<Story name="Forbidden" args={{ content: content(403, 'forbidden') }} />

<Story name="Service error" args={{ content: content(500, 'internal') }} />

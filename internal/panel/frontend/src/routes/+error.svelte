<script lang="ts">
  import { page } from '$app/state';

  import { createPanelApi } from '#lib/api.js';
  import { readPanelBuild } from '#lib/base.js';
  import ErrorPage from '#lib/components/ErrorPage.svelte';
  import { basePath } from '#lib/paths.js';
  import { getPanelSession } from '#lib/session.svelte.js';

  const session = getPanelSession();
  const api = createPanelApi(basePath, (input, init) => fetch(input, init));
  const build = readPanelBuild(document);
  const insidePanel = $derived(session.viewer !== null && !session.isInvitation);
  const failure = $derived({
    status: page.status,
    code: '',
    message: page.error?.message ?? 'Something went wrong',
  });

  /**
   * Where a reader who is already inside can go instead.
   *
   * Named here rather than in the page, because which pages exist is the session's
   * to know: a reader whose workspace has no plan waiting is not offered one.
   */
  const destinations = $derived(
    !insidePanel || session.selectedTarget === null
      ? []
      : [
          { label: 'Repositories', href: session.viewHref('repositories') },
          { label: 'Queue', href: session.queueSectionHref('active') },
          { label: 'Sync status', href: session.syncSectionHref('overview') },
        ],
  );
</script>

<ErrorPage {api} base={basePath} {build} {failure} {insidePanel} {destinations} />

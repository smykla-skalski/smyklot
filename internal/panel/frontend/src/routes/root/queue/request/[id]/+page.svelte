<script lang="ts">
  import { page } from '$app/state';
  import QueueRequest from '$lib/components/QueueRequest.svelte';
  import { getPanelSession } from '$lib/session.svelte';

  const session = getPanelSession();
  const requestId = $derived(page.params.id ?? '');
</script>

<section class="root-workspace" aria-labelledby="root-page-heading">
  <QueueRequest
    api={session.api}
    rootRole={session.rootRole}
    {requestId}
    queueHref={session.queueHref()}
    onBack={() => session.selectQueueSection('waiting')}
  />
</section>

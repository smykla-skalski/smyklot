<script lang="ts">
  import QueueRequest from '#lib/components/QueueRequest.svelte';
  import { getPanelSession } from '#lib/session.svelte.js';

  import type { PageProps } from './$types';

  const { params }: PageProps = $props();
  const session = getPanelSession();
</script>

<section class="root-workspace" aria-labelledby="root-page-heading">
  <QueueRequest
    api={session.api}
    rootRole={session.rootRole}
    requestId={params.id}
    queueHref={session.queueHref()}
    onBack={() => session.selectQueueSection('waiting')}
  />
</section>

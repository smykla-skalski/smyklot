<script lang="ts">
  import QueueRequest from '#lib/components/QueueRequest.svelte';
  import { getPanelSession } from '#lib/session.svelte.js';

  import type { PageProps } from './$types';

  const { params }: PageProps = $props();
  const session = getPanelSession();
</script>

<div class="root-workspace">
  <QueueRequest
    api={session.api}
    requestId={params.id}
    queueHref={session.queueHref()}
    onBack={() => session.selectRootQueueSection('active')}
  />
</div>

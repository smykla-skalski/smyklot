<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { PanelApiError } from '../api';
  import { queueDetailKey } from '../queue-cache';
  import type { QueueDetail } from '../types';
  import QueueDetailDialog from './QueueDetailDialog.svelte';

  const {
    itemId,
    targetId,
    fetchItem,
    onClose,
  }: {
    itemId: string | null;
    targetId?: string;
    fetchItem: (id: string) => Promise<QueueDetail>;
    onClose: () => void;
  } = $props();

  let followed = $state<{ origin: string; id: string } | null>(null);
  const currentId = $derived(followed?.origin === itemId ? followed.id : itemId);
  function close() {
    followed = null;
    onClose();
  }

  const query = createQuery(() => ({
    queryKey: queueDetailKey(targetId, currentId ?? ''),
    queryFn: () => {
      if (currentId === null) throw new Error('No queue item selected');
      return fetchItem(currentId);
    },
    enabled: itemId !== null,
    staleTime: 0,
    retry: false,
    refetchInterval: (query) => {
      const data = query.state.data;
      const state =
        data?.delivery?.current?.queue?.state ??
        data?.delivery?.current?.status ??
        data?.item.state;
      return itemId !== null &&
        state !== undefined &&
        !['failed', 'cancelled', 'superseded', 'succeeded'].includes(state)
        ? 15_000
        : false;
    },
  }));
  const unavailable = $derived(
    query.error instanceof PanelApiError && [401, 403, 404].includes(query.error.status),
  );
  const error = $derived.by(() => {
    const cause = query.error;
    if (cause === null) return '';
    if (cause instanceof PanelApiError) {
      if (cause.status === 404)
        return 'This queue record is no longer available. Close this inspector to return to the original record.';
      if (cause.status === 401 || cause.status === 403)
        return 'You no longer have access to this queue record. Close this inspector and check your account access.';
    }
    return 'The queue record could not be loaded. Try again.';
  });
</script>

<!--
@component
Loads one queue record for both queue rows and failure history. The target scope
is part of its shared cache key; root readers omit it. Opening revalidates the
record, including cached records which may have expired or changed since last read.
A missing or inaccessible record stays in the inspector so the reader can return
to the original context. Temporary failures offer an explicit retry.
-->

<QueueDetailDialog
  open={itemId !== null}
  detail={query.data ?? null}
  loading={query.isFetching}
  {error}
  onRetry={error !== '' && !unavailable ? () => void query.refetch() : undefined}
  onClose={close}
  onInspectItem={(id) => {
    if (itemId !== null) followed = { origin: itemId, id };
  }}
/>

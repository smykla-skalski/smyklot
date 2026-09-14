<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { PanelApiError, type PanelApi } from '../api';
  import { queueDetailKey } from '../queue-cache';
  import type { QueueDetail } from '../types';
  import DeliveryRetry from './DeliveryRetry.svelte';
  import QueueDetailDialog from './QueueDetailDialog.svelte';
  import SyncCheckEvidence from './SyncCheckEvidence.svelte';
  import { checkOutcome } from '../sync-check';

  const {
    itemId,
    recoveryApi,
    checkEvidenceApi,
    targetId,
    fetchItem,
    onClose,
    syncResultHref,
  }: {
    itemId: string | null;
    checkEvidenceApi?: Pick<PanelApi, 'fetchSyncCheckObservations'>;
    recoveryApi?: Pick<PanelApi, 'previewDeliveryRecovery' | 'retryDelivery'>;
    targetId?: string;
    fetchItem: (id: string) => Promise<QueueDetail>;
    onClose: () => void;
    syncResultHref?: (id: string) => string;
  } = $props();

  const client = useQueryClient();
  let recoveryPending = $state(false);
  let followed = $state<{ origin: string; id: string } | null>(null);
  const currentId = $derived(followed?.origin === itemId ? followed.id : itemId);
  function close() {
    if (recoveryPending) return;
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
  {syncResultHref}
  open={itemId !== null}
  detail={query.data ?? null}
  loading={query.isFetching}
  {error}
  onRetry={error !== '' && !unavailable ? () => void query.refetch() : undefined}
  onRefresh={!unavailable ? () => void query.refetch() : undefined}
  onClose={close}
  {recoveryPending}
  onInspectItem={(id) => {
    if (!recoveryPending && itemId !== null) followed = { origin: itemId, id };
  }}
>
  {#snippet checkEvidence()}
    {#if checkEvidenceApi && query.data?.item.kind === 'sync_scan' && query.data.item.target_id && checkOutcome(query.data.item.details?.outcome)}
      {#key `${query.data.item.target_id}:${query.data.item.id}`}
        <SyncCheckEvidence
          api={checkEvidenceApi}
          targetId={query.data.item.target_id}
          checkId={query.data.item.id}
        />
      {/key}
    {/if}
  {/snippet}
  {#snippet recovery()}
    {#if recoveryApi && query.data?.item.kind === 'webhook_delivery' && query.data.item.state === 'failed' && query.data.item.target_id && query.data.item.source_id}
      {#key `${targetId ?? 'root'}:${query.data.item.id}`}
        <DeliveryRetry
          api={recoveryApi}
          targetId={query.data.item.target_id}
          deliveryId={query.data.item.source_id}
          root={targetId === undefined}
          onPendingChange={(pending) => {
            recoveryPending = pending;
          }}
          onAccepted={(id) => {
            if (itemId !== null) followed = { origin: itemId, id };
            void client.invalidateQueries({ queryKey: ['queue'] });
            void client.invalidateQueries({ queryKey: ['root-overview'] });
          }}
        />
      {/key}
    {/if}
  {/snippet}
</QueueDetailDialog>

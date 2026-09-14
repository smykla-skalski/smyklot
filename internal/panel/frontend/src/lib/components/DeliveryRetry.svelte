<script lang="ts">
  import { untrack } from 'svelte';
  import { useQueryClient } from '@tanstack/svelte-query';
  import { PanelApiError, type PanelApi } from '../api';
  import {
    recoveryExplanation,
    recoveryRequest,
    type DeliveryRecoveryPreview,
    type DeliveryRecoveryRequest,
  } from '../delivery-recovery';
  import Button from './Button.svelte';

  const {
    api,
    targetId,
    deliveryId,
    root,
    onAccepted,
    onPendingChange,
  }: {
    api: Pick<PanelApi, 'previewDeliveryRecovery' | 'retryDelivery'>;
    targetId: string;
    deliveryId: string;
    root: boolean;
    onAccepted: (queueId: string) => void;
    onPendingChange: (pending: boolean) => void;
  } = $props();
  const client = useQueryClient();
  const cacheKey = $derived(['delivery-recovery-request', root, targetId, deliveryId]);
  const saved = untrack(() => client.getQueryData<DeliveryRecoveryRequest>(cacheKey));
  let intent = $state.raw<DeliveryRecoveryRequest | null>(saved ?? null);
  let expanded = $state(saved !== undefined);
  let pending = $state(false);
  let preview = $state.raw<DeliveryRecoveryPreview | null>(null);
  let problem = $state(saved ? 'A previous retry request needs its result checked.' : '');

  function busy(value: boolean) {
    pending = value;
    onPendingChange(value);
  }
  async function check() {
    if (intent) {
      expanded = true;
      return;
    }
    expanded = true;
    problem = '';
    busy(true);
    try {
      preview = await api.previewDeliveryRecovery(targetId, deliveryId, root);
    } catch {
      preview = null;
      problem = 'Recovery could not be checked. Try again.';
    } finally {
      busy(false);
    }
  }
  async function submit() {
    if (pending || (!intent && !preview?.available)) return;
    const request = intent ?? recoveryRequest(preview!, crypto.randomUUID());
    intent = request;
    // An unknown outcome must outlive the usual inactive-query eviction window.
    client.setQueryDefaults(cacheKey, { gcTime: Infinity });
    client.setQueryData(cacheKey, request);
    problem = '';
    busy(true);
    try {
      const result = await api.retryDelivery(targetId, deliveryId, root, request);
      client.removeQueries({ queryKey: cacheKey, exact: true });
      intent = null;
      onAccepted(result.queue_id);
    } catch (error) {
      if (error instanceof PanelApiError && error.status >= 400 && error.status < 500) {
        client.removeQueries({ queryKey: cacheKey, exact: true });
        intent = null;
        preview = null;
        problem = error.message;
      } else {
        problem =
          'The retry result could not be confirmed. Check its result before requesting another run.';
      }
    } finally {
      busy(false);
    }
  }
</script>

<!--
@component
Reviews and submits one delivery recovery. Key by target, original delivery and
Root scope so state never crosses subjects. Unknown submission outcomes retain
the exact request in the shared query cache across inspector dismissal. Checking
that result repeats the same key and preconditions rather than starting new work.
The inspector guards dismissal while a request is pending.
-->

<div class="delivery-retry">
  {#if !expanded}
    <Button onclick={() => void check()}>Review retry</Button>
  {:else}
    <div aria-live="polite" aria-busy={pending}>
      {#if pending && preview === null && !intent}
        <p>Checking recovery…</p>
      {:else if problem}
        <p role="alert">{problem}</p>
      {:else if preview?.available}
        <p>{preview.effect}</p>
        <p>The original failure stays in history. Retrying does not undo earlier GitHub changes.</p>
      {:else if preview}
        <p>{recoveryExplanation(preview.reason)}</p>
      {/if}
    </div>
    <div class="retry-actions">
      {#if pending && intent}
        <Button tone="signal" disabled>Requesting retry…</Button>
      {:else if intent}
        <Button tone="signal" disabled={pending} onclick={() => void submit()}
          >Check retry result</Button
        >
      {:else if preview?.available}
        <Button tone="signal" disabled={pending} onclick={() => void submit()}>Start retry</Button>
      {:else}
        <Button disabled={pending} onclick={() => void check()}>Check availability</Button>
      {/if}
      <Button
        disabled={pending}
        onclick={() => {
          expanded = false;
        }}>Back</Button
      >
    </div>
  {/if}
</div>

<style>
  .delivery-retry {
    margin-top: var(--space-3);
  }
  p {
    margin: 0 0 var(--space-3);
  }
  .retry-actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
</style>

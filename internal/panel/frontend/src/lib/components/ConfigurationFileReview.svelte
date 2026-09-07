<script lang="ts">
  import { onDestroy } from 'svelte';
  import { BOOLEAN_FIELDS } from '../config';
  import {
    ConfigurationReview,
    type ConfigurationReviewSource,
  } from '../config-file-review.svelte';
  import { configFileProposalHref } from '../config-file-status';
  import { formatJson } from '../merge';
  import { templateBody } from '../template-content';
  import Modal from './Modal.svelte';
  import Button from './Button.svelte';
  import IconButton from './IconButton.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import CodeEditor from './CodeEditor.svelte';
  import DiffBlock from './DiffBlock.svelte';
  import FormError from './FormError.svelte';
  import Callout from './Callout.svelte';
  import Link from './Link.svelte';

  const { source, repository }: { source?: ConfigurationReviewSource; repository: string } =
    $props();
  const review = new ConfigurationReview(() => source);
  let trigger = $state<HTMLElement | null>(null);
  let chosenView = $state<{ token: string; value: string } | null>(null);
  const preview = $derived(review.preview);
  const removed = $derived(preview?.problem === 'file_removed');
  const choices = $derived(
    preview?.choices?.filter((choice) => choice.available && choice.document !== undefined) ?? [],
  );
  const choiceOptions = $derived(
    choices.map((choice) => ({
      value: choice.side,
      label: choice.side === 'panel' ? 'Panel values' : 'File values',
    })),
  );
  const proposalHref = $derived(configFileProposalHref(preview?.proposal, repository));
  const panelChoice = $derived(choices.find((choice) => choice.side === 'panel'));
  const fileChoice = $derived(choices.find((choice) => choice.side === 'file'));
  const comparable = $derived(!removed && panelChoice !== undefined && fileChoice !== undefined);
  const panelText = $derived(
    panelChoice?.document === undefined ? '' : formatJson(panelChoice.document),
  );
  const fileText = $derived(
    fileChoice?.document === undefined ? '' : formatJson(fileChoice.document),
  );
  const view = $derived(
    !comparable
      ? 'result'
      : chosenView?.token === preview?.review_token && review.choice !== null
        ? (chosenView?.value ?? 'conflicts')
        : 'conflicts',
  );
  const text = $derived(
    review.choice?.document === undefined ? '' : formatJson(review.choice.document),
  );
  const explanation = $derived.by(() => {
    if (removed)
      return 'The connected file was deleted · recreating it requires your explicit choice';
    if (preview?.problem === 'conflicting_edits')
      return 'Review the overlapping changes, then choose which values to keep · independent edits from both sides are preserved';
    if (preview?.message) return preview.message.replace(/[.]+$/u, '');
    if (preview?.status === 'ready') return 'The saved settings match the file';
    if (preview?.status === 'pending') return 'Changes have not finished syncing';
    return 'This comparison does not offer a change that can be applied';
  });

  export function openReview(element?: HTMLElement): void {
    trigger = element ?? null;
    chosenView = null;
    void review.show();
  }

  function close(): void {
    review.close();
    queueMicrotask(() => {
      if (trigger?.isConnected) trigger.focus({ preventScroll: true });
    });
  }
  function settingName(path: string[]): string | null {
    if (path.length === 1) {
      if (path[0] === 'command_prefix') return 'Command prefix';
      if (path[0] === 'command_aliases') return 'Command aliases';
      if (path[0] === 'allowed_commands') return 'Allowed commands';
      const field = BOOLEAN_FIELDS.find((field) => field.key === path[0]);
      if (field) return field.label;
    }
    return null;
  }
  function pointer(path: string[]): string {
    return path.length === 0
      ? '(whole document)'
      : '/' + path.map((part) => part.replaceAll('~', '~0').replaceAll('/', '~1')).join('/');
  }

  $effect(() => review.reconcile());
  onDestroy(() => review.close());
</script>

<!--
@component
A fresh comparison of saved settings and the connected file. Choosing a view only
previews its complete result; the footer sends the current token and explicit side.
The owner provides scope, authority and draft guards. Closing invalidates requests.
-->
<Modal
  id="configuration-file-review"
  open={review.open}
  title="Review configuration file"
  description={preview?.path ? `${repository} · ${preview.path}` : repository}
  variant="inspector"
  returnFocus={trigger}
  onClose={close}
>
  {#snippet headerExtra()}<IconButton
      toolbar
      icon="close"
      label="Close configuration file review"
      onclick={close}
    />{/snippet}
  <div class="form-stack" aria-busy={review.loading || review.saving}>
    {#if review.blocker}
      <Callout><p>{review.blocker}</p></Callout>
    {:else if review.pending}
      <Callout role="status"><p>Your choice was accepted · changes are waiting to sync</p></Callout>
    {:else if review.loading}
      <p class="form-help" role="status">Comparing saved settings with the file</p>
    {:else}
      {#if review.notice}<Callout role="status"><p>{review.notice}</p></Callout>{/if}
      <FormError message={review.problem} />
      {#if preview}
        <p class="form-help">{explanation}</p>
        {#if preview.conflict_paths?.length}
          <div class="form-field">
            <span class="form-label"
              >{preview.conflict_count ?? preview.conflict_paths.length} conflicting {(preview.conflict_count ??
                preview.conflict_paths.length) === 1
                ? 'setting'
                : 'settings'}</span
            >
            <ul class="review-paths form-field" aria-label="Overlapping settings">
              {#each preview.conflict_paths as path (JSON.stringify(path))}
                <li class="form-help review-path">
                  {#if settingName(path)}<span>{settingName(path)}</span>{/if}<code
                    >{pointer(path)}</code
                  >
                </li>
              {/each}
            </ul>
          </div>
        {/if}
        {#if proposalHref}<p class="form-help">
            <Link href={proposalHref} target="_blank" rel="noreferrer"
              >Review pull request #{preview.proposal!.number}</Link
            >
          </p>{/if}
        {#if choices.length}
          {#if !removed}<div class="form-field">
              <div class="form-row">
                <span class="form-label">Keep values from</span>
                <SegmentedControl
                  name="configuration-file-choice"
                  label="Keep values from"
                  value={review.selected}
                  options={choiceOptions}
                  disabled={review.saving}
                  onSelect={(side) => review.select(side)}
                />
              </div>
              {#if !review.choice && !comparable}
                <p class="form-help">Choose Panel values or File values to preview the result</p>
              {/if}
            </div>{/if}
          {#if comparable || review.choice}
            <div class="review-code">
              <div class="card-head">
                {#if comparable}
                  <SegmentedControl
                    name="configuration-file-view"
                    label="Configuration file view"
                    value={view}
                    options={[
                      { value: 'conflicts', label: 'Conflicts' },
                      {
                        value: 'result',
                        label: 'Full result',
                        disabled: review.choice === null,
                      },
                    ]}
                    onSelect={(value) =>
                      (chosenView = { token: preview.review_token ?? '', value })}
                  />
                {:else}<h3 class="card-title">Resulting settings</h3>{/if}
                <span class="card-meta">JSON · Read only</span>
              </div>
              {#if view === 'conflicts'}
                <DiffBlock
                  before={templateBody(panelText)}
                  after={templateBody(fileText)}
                  contextLines={2}
                  label="Conflicting settings"
                  labels={{ before: 'Panel values', after: 'File values' }}
                />
              {:else}
                <CodeEditor
                  value={text}
                  readOnly
                  terminalNewline
                  label="Resulting settings"
                  onChange={() => {}}
                />
              {/if}
            </div>
          {/if}
        {/if}
        {#each preview.choices?.filter((choice) => !choice.available) ?? [] as choice (choice.side)}
          <FormError
            message={choice.message
              ? `${choice.side === 'panel' ? 'Panel' : 'File'} values: ${choice.message.replace(/[.]+$/u, '')}`
              : `${choice.side === 'panel' ? 'Panel' : 'File'} values cannot produce valid settings`}
          />
        {/each}
        {#if !source?.canWrite}<p class="form-help">
            Read only · write access is required to apply a choice
          </p>{/if}
      {/if}
    {/if}
  </div>
  {#snippet footer()}
    {#if !review.pending && !review.blocker && !review.loading && !choices.length}
      <Button tone="quiet" disabled={review.saving} onclick={() => void review.refresh()}
        >Refresh comparison</Button
      >
    {/if}
    <Button tone="quiet" onclick={close}>Done</Button>
    {#if choices.length && source?.canWrite && !review.pending}
      <Button
        tone="brand"
        disabled={!review.canSubmit}
        aria-busy={review.saving}
        onclick={() => void review.submit()}
        >{review.saving
          ? 'Applying choice'
          : removed
            ? 'Recreate file'
            : review.selected === 'file'
              ? 'Use file values'
              : review.selected === 'panel'
                ? 'Use panel values'
                : 'Apply choice'}</Button
      >
    {/if}
  {/snippet}
</Modal>

<style>
  .review-code {
    min-inline-size: 0;
  }
  .review-code .card-title {
    font-size: var(--font-size-meta);
  }
  .review-paths {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .review-path {
    display: flex;
    flex-wrap: wrap;
    gap: var(--row-copy-gap);
    overflow-wrap: anywhere;
  }
</style>

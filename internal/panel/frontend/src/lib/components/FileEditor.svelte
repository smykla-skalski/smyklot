<script lang="ts">
  import type { CodeLang } from '../code-tokens';
  import { templateBody, terminateTemplate } from '../template-content';
  import CodeBlock from './CodeBlock.svelte';
  import CodeEditor from './CodeEditor.svelte';
  import DiffBlock from './DiffBlock.svelte';
  import FormError from './FormError.svelte';
  import IconButton from './IconButton.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  const {
    value,
    output,
    lang,
    readOnly = false,
    busy = false,
    problem = '',
    label = 'Template',
    comparison,
    overridden,
    onChange,
    onFormat,
    onOptions,
  }: {
    value: string;
    output: string | null;
    lang: CodeLang;
    readOnly?: boolean;
    busy?: boolean;
    problem?: string;
    label?: string;
    comparison?: string;
    overridden?: ReadonlySet<number> | null;
    onChange: (value: string) => void;
    onFormat?: () => void;
    onOptions?: (trigger: HTMLElement) => void;
  } = $props();

  let mode = $state('edit');
  let historyDepth = $state(0);
  let editor = $state<CodeEditor | null>(null);
  const before = $derived(terminateTemplate(comparison ?? value));
  const after = $derived(output === null ? null : terminateTemplate(output));
  const differs = $derived(after !== null && templateBody(before) !== templateBody(after));

  export function replaceValue(next: string): void {
    editor?.replaceValue(next);
    mode = 'edit';
  }
</script>

<!--
@component
One visible code surface. Keep the editor mounted behind preview so selection,
     scroll position and undo history survive a look at the resulting file. -->
<div class="file-editor">
  <div class="card-head">
    <h2 class="card-title">{label}</h2>
    <div class="editor-actions">
      {#if historyDepth > 0}
        <IconButton
          toolbar
          icon="undo"
          label="Undo"
          onclick={() => {
            mode = 'edit';
            editor?.undoEdit();
          }}
        />
      {/if}
      <SegmentedControl
        name="file-editor-view-{label}"
        label="{label} view"
        value={mode}
        options={[
          { value: 'edit', label: readOnly ? 'Source' : 'Edit' },
          { value: 'preview', label: 'Preview' },
        ]}
        onSelect={(value) => (mode = value)}
      />
      {#if onOptions}
        <IconButton
          toolbar
          icon="sliders"
          label="{label} options"
          onclick={(event) => onOptions(event.currentTarget as HTMLElement)}
        />
      {/if}
    </div>
  </div>
  {#if problem}<FormError message={problem} />{/if}
  {#if mode === 'preview'}
    <span class="visually-hidden" role="status"
      >{busy ? 'Updating preview' : 'Read-only preview'}</span
    >
    {#if after !== null}
      <div
        class="file-preview"
        class:refreshing={busy}
        role="region"
        aria-label="Read-only final output with highlighted changes"
      >
        {#if differs}
          <DiffBlock {before} {after} {lang} />
        {:else}
          <CodeBlock text={after} {lang} />
        {/if}
      </div>
    {:else if !problem}
      <p class="preview-state" role="status">Preparing preview…</p>
    {/if}
  {/if}
  <div hidden={mode !== 'edit'}>
    <CodeEditor
      bind:this={editor}
      {value}
      {lang}
      {readOnly}
      {overridden}
      {onChange}
      {onFormat}
      terminalNewline
      onHistory={(depth) => (historyDepth = depth)}
    />
  </div>
</div>

<style>
  .file-editor {
    min-inline-size: 0;
  }
  .editor-actions {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .editor-actions > :global(*) {
    margin-block: calc((var(--card-head-line) - var(--control-height-compact)) / 2);
  }
  .preview-state {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    margin: 0;
  }
  .refreshing {
    opacity: 0.6;
  }
  @media (max-width: 47.9375rem) {
    .card-head {
      flex-wrap: wrap;
      row-gap: var(--space-4);
    }
    .editor-actions {
      margin-inline-start: auto;
    }
  }
</style>

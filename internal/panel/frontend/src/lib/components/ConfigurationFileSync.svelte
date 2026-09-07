<script lang="ts">
  import {
    configFileProposalHref,
    configFileStatusView,
    type ConfigFileStatusConnection,
  } from '../config-file-status';
  import Card from './Card.svelte';
  import Switch from './Switch.svelte';
  import Pill from './Pill.svelte';
  import Button from './Button.svelte';
  import Link from './Link.svelte';
  import RelativeTime from './RelativeTime.svelte';

  const {
    scope,
    repository,
    enabled,
    savedEnabled,
    fileIgnored = false,
    savedFileIgnored = false,
    dirty = false,
    readOnly = false,
    connection,
    id,
    now,
    onChange,
  }: {
    scope: 'workspace' | 'repository';
    /** Known GitHub owner/repository, including .github for workspace settings. */
    repository: string;
    enabled: boolean;
    savedEnabled: boolean;
    fileIgnored?: boolean;
    savedFileIgnored?: boolean;
    dirty?: boolean;
    readOnly?: boolean;
    connection?: ConfigFileStatusConnection;
    id?: string;
    now: number;
    onChange: (enabled: boolean) => void;
  } = $props();

  const view = $derived(
    connection?.data ? configFileStatusView(connection.data, savedEnabled, savedFileIgnored) : null,
  );
  const check = $derived(
    connection?.data?.enabled === savedEnabled ? connection?.data?.last_check : undefined,
  );
  const proposalHref = $derived(configFileProposalHref(view?.proposal, repository));
  const draftHint = $derived(
    dirty ? (enabled ? 'Starts after you save' : 'Stops after you save') : null,
  );
</script>

<!--
@component
The editable opt-in and the saved connection are separate facts. The parent owns
reads and draft persistence; this card cannot start a sync or resolve a conflict.
-->
<Card {id} label="Configuration file sync">
  <div class="card-head">
    <h2 class="card-title">Configuration file sync</h2>
    {#if readOnly}<span class="card-meta">Read only</span>{/if}
  </div>
  <div class="policy-rows">
    <div class={['policy-row', { 'is-unsaved': dirty }]} data-unsaved={dirty || undefined}>
      <span class="setting-say">
        <span class="setting-name">Sync settings with file</span>
        <span class="setting-why"
          >Panel changes open pull requests · file changes on the default branch update these
          settings</span
        >
        {#if draftHint}<span class="setting-why">{draftHint}</span>{/if}
        {#if scope === 'repository' && enabled && fileIgnored}
          <span class="setting-why"
            >Use file settings must also be on to allow imports and pull requests</span
          >
        {/if}
      </span>
      <span class="policy-value">
        <Switch
          bare
          checked={enabled}
          disabled={readOnly}
          label="Sync settings with file"
          onToggle={onChange}
        />
      </span>
    </div>
    {#if scope === 'workspace'}
      <div class="policy-row">
        <span class="setting-say"
          ><span class="setting-name">Workspace file</span><span class="setting-why"
            >In {repository} · separate from that repository’s own settings file</span
          ></span
        >
        <span class="policy-value"
          ><code class="setting-fact file-path">.smyklot/workspace.toml</code></span
        >
      </div>
    {:else if check?.path}
      <div class="policy-row">
        <span class="setting-name">Connected file</span>
        <span class="policy-value"><code class="setting-fact file-path">{check.path}</code></span>
      </div>
    {/if}
    <div class="policy-row" aria-label="Saved connection status">
      <span class="setting-say">
        <span class="setting-name">Sync status</span>
        <span class="setting-why">
          {#if connection?.error}Status could not be loaded
          {:else if view}{view.description}
          {:else if connection?.isPending}Loading status
          {:else}Connection status is not available here{/if}
        </span>
        {#if connection?.error}<span class="setting-why"
            >{connection.error.message.replace(/[.]+$/u, '')}</span
          >{/if}
      </span>
      <span class="policy-value">
        {#if connection?.error}
          <Button
            tone="quiet"
            disabled={connection.isFetching}
            onclick={() => connection?.refetch()}>Try again</Button
          >
        {:else if view}<Pill tone={view.tone}>{view.label}</Pill>{/if}
      </span>
    </div>
    {#if proposalHref && view?.proposal}
      <div class="policy-row">
        <span class="setting-name">GitHub pull request</span>
        <span class="policy-value"
          ><Link href={proposalHref} target="_blank" rel="noreferrer">#{view.proposal.number}</Link
          ></span
        >
      </div>
    {/if}
    {#if check}
      <div class="policy-row">
        <span class="setting-name">Last checked</span>
        <span class="policy-value"
          ><RelativeTime class="setting-fact" value={check.checked_at} nowMs={now} /></span
        >
      </div>
    {/if}
  </div>
</Card>

<style>
  .file-path {
    overflow-wrap: anywhere;
  }
</style>

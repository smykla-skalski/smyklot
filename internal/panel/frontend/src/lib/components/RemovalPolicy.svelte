<script lang="ts">
  /**
   * What happens to the things a list does not name: the one destructive switch
   * a kind has, and the patterns that are exempt from it.
   *
   * The two belong together and are one card wherever a kind keeps a list.
   * Somebody who can turn removal on from a page has to be able to protect
   * something from that page too, which is the whole reason the exemptions are
   * here rather than only in the API - and separating them puts the destructive
   * switch on one card and its safety valve on another.
   *
   * Read after the list rather than above it: these are settings about the list,
   * and above it they put the destructive switch first and the thing it destroys
   * second.
   */
  import PatternList from './PatternList.svelte';
  import PolicyRow from './PolicyRow.svelte';
  import Plate from './Plate.svelte';
  import Switch from './Switch.svelte';

  const {
    noun,
    removal,
    excludes,
    disabled = false,
    onRemovalChange,
    onExcludesChange,
  }: {
    /** What the list holds, in the plural: `labels`, `rulesets`. */
    noun: string;
    removal: boolean;
    excludes: readonly string[];
    disabled?: boolean;
    onRemovalChange: (next: boolean) => void;
    onExcludesChange: (next: string[]) => void;
  } = $props();

  const removalName = $derived(`Remove ${noun} this list does not name`);
  const exemptName = $derived(`${noun[0]?.toUpperCase()}${noun.slice(1)} to leave alone`);
</script>

<Plate label="How the list is applied">
  <div class="removal-policy">
    <PolicyRow
      name={removalName}
      why="Off, a repository may keep {noun} of its own. On, everything unnamed is deleted"
      value={removal ? 'On' : 'Off'}
    >
      {#snippet control()}
        <!-- The one control here that destroys something. Anything the list
             does not name goes on existing unless this is on. -->
        <Switch checked={removal} ariaLabel={removalName} {disabled} onChange={onRemovalChange} />
      {/snippet}
    </PolicyRow>

    <PolicyRow
      name={exemptName}
      why="Name or pattern, where * stands for any run of characters. Neither written nor removed, whatever the switch above says"
    >
      {#snippet control()}
        <PatternList
          values={excludes}
          label={exemptName}
          addLabel="Add a pattern"
          placeholder="hand-made-*"
          {disabled}
          onChange={onExcludesChange}
        />
      {/snippet}
    </PolicyRow>
  </div>
</Plate>

<style>
  .removal-policy {
    display: grid;
  }

  .removal-policy > :global(.policy-row + .policy-row) {
    border-top: 1px solid var(--border-subtle);
  }
</style>

<script lang="ts">
  import { monogram } from '../lib/identity';
  import type { PanelAccount } from '../lib/types';

  const {
    account,
    size,
  }: {
    account: PanelAccount;
    size: number;
  } = $props();

  // A profile picture the browser cannot fetch leaves a broken-image glyph where
  // a face belongs, and the avatar host is one the panel does not control.
  //
  // Recorded as the URL that failed rather than a flag, because rows are keyed by
  // account and survive a refresh: a flag would hold the monogram for the life of
  // the row, so a new profile picture would never be tried. Matching on the URL
  // retries a changed one while still not re-requesting the one that just failed.
  let failed = $state<string | null>(null);

  const source = $derived(account.avatar_url === failed ? null : account.avatar_url);
</script>

<!-- Decorative: the name it belongs to is always beside it, so announcing the
     picture as well would read the same account twice. The referrer is withheld
     because the avatar host has no business learning the panel's address. -->
{#if source !== null}
  <img
    class="avatar"
    style="--avatar-size: {size}px"
    src={source}
    alt=""
    width={size}
    height={size}
    loading="lazy"
    decoding="async"
    referrerpolicy="no-referrer"
    onerror={() => (failed = account.avatar_url)}
  />
{:else}
  <span class="avatar avatar-fallback" style="--avatar-size: {size}px" aria-hidden="true">
    {monogram(account.display_name, account.login)}
  </span>
{/if}

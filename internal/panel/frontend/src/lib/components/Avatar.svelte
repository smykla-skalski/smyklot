<script lang="ts">
  import { monogram } from '../identity';
  import type { PanelAccount } from '../types';

  const {
    account,
    size,
    shape = 'person',
  }: {
    account: PanelAccount;
    size: number;
    /**
     * Who or what the picture stands for, which decides its outline: a person is
     * a circle and a workspace is a rounded square.
     *
     * The distinction is GitHub's own - it draws organisations square and people
     * round - and the panel already keeps it, in the rounded square the Root
     * console puts an installation's monogram in. It earns its place in the top
     * bar on a phone, where the workspace switcher and the account menu lose
     * their labels and stand next to each other as two identical discs.
     */
    shape?: 'person' | 'workspace';
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

  // Initials snap to a type token rather than scaling with the circle: a 28px
  // avatar carries micro, a 32px one carries meta. Scaling by a ratio landed
  // them between sizes, which reads as a slightly different weight in every
  // list. The map lives here so no call site picks its own.
  // A 24px circle was the smallest the map knew about, and everything under it
  // took the same 11px: two capitals at that size fill a 20px circle to its ring
  // and read as letters escaping it. Nano is the step below.
  const monogramFont = $derived(
    size < 24
      ? 'var(--font-size-nano)'
      : size < 32
        ? 'var(--font-size-micro)'
        : size < 40
          ? 'var(--font-size-meta)'
          : 'var(--font-size-title)',
  );
</script>

<!-- Decorative: the name it belongs to is always beside it, so announcing the
     picture as well would read the same account twice. The referrer is withheld
     because the avatar host has no business learning the panel's address.

     Sized through `style:` rather than a `style` attribute. The panel serves
     `style-src 'self'`, which drops a parsed style attribute outright, and the
     directive is applied through the CSSOM instead - see the note in app.css. -->
{#if source !== null}
  <img
    class="avatar"
    class:avatar-workspace={shape === 'workspace'}
    style:--avatar-size="{size}px"
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
  <span
    class="avatar avatar-fallback"
    class:avatar-workspace={shape === 'workspace'}
    style:--avatar-size="{size}px"
    style:--avatar-font={monogramFont}
    aria-hidden="true"
  >
    <!-- Trimmed to the caps so the initials centre on their own ink, not on a
         line box with room for descenders these two letters never have. -->
    <span class="cap-trim">{monogram(account.display_name, account.login)}</span>
  </span>
{/if}

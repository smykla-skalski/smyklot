<script lang="ts">
  /**
   * The page the panel shows while it does not yet know which page it is.
   *
   * Signing in is answered by a request, and until that answer arrives there are
   * two possible layouts and no way to choose between them. The layout used to
   * choose anyway: `signedOut` is `!loading && viewer === null`, which is false
   * while the answer is outstanding, so every load fell through to the shell -
   * sidebar, workspace, footer and all - and a reader who turned out to be
   * signed out watched it be replaced by the sign-in page a moment later.
   *
   * So this is what "not known yet" looks like, and it deliberately looks like
   * neither destination: no shell chrome to be taken away, no sign-in card to be
   * offered and withdrawn. The ground is already the reader's theme - `boot.ts`
   * applies it before this mounts - so the only thing that changes when the
   * answer lands is what stands on it.
   *
   * Silent for a moment before it says anything. Most answers arrive inside it,
   * and a status line that appears and vanishes inside 300ms is a flash of its
   * own; past that the wait is real and worth acknowledging.
   */
</script>

<div class="panel-boot" role="status" aria-live="polite">
  <span class="panel-boot-word">Loading</span>
</div>

<style>
  .panel-boot {
    align-items: center;
    display: flex;
    justify-content: center;
    min-height: 100dvh;
  }

  .panel-boot-word {
    animation: panel-boot-appear var(--duration-normal) var(--ease-standard) 300ms both;
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    text-box: trim-both cap alphabetic;
  }

  @keyframes panel-boot-appear {
    from {
      opacity: 0;
    }

    to {
      opacity: 1;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .panel-boot-word {
      animation-duration: 1ms;
    }
  }
</style>

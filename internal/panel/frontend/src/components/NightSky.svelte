<script lang="ts">
  /**
   * A patch of sky that opens out from a point and fades into the page.
   *
   * Drawn entirely from gradients: no raster, nothing to load, and it resamples
   * at any size or pixel density. The stars are tiled layers rather than one
   * gradient per star - three tiles whose edges share no common factor, so the
   * repeat has no beat you can pick out inside the circle the mask leaves.
   */
  const {
    size = '40rem',
  }: { /** Diameter of the sky, before the mask fades it. */ size?: string } = $props();
</script>

<span class="night-sky" style="--sky-size: {size}" aria-hidden="true">
  <span class="sky-wash"></span>
  <span class="sky-far"></span>
  <span class="sky-near"></span>
</span>

<style>
  .night-sky {
    /* Light: the sky is a breath of cool over the canvas, and the stars are
       specks of the same ink the page writes in - a constellation rather than a
       dark patch cut out of a bright page. */
    --sky-core: rgb(9 21 43 / 7%);
    --sky-glow: rgb(109 84 189 / 9%);
    --sky-star-bright: rgb(16 30 56 / 58%);
    --sky-star: rgb(28 44 72 / 40%);
    --sky-dust: rgb(38 56 86 / 26%);

    aspect-ratio: 1;
    display: block;
    /* The mask is what makes it a sky rather than a disc: opaque through the
       middle, then a long ramp to nothing so the edge never lands anywhere. */
    mask-image: radial-gradient(
      circle at 50% 50%,
      rgb(0 0 0 / 100%) 0%,
      rgb(0 0 0 / 100%) 26%,
      rgb(0 0 0 / 62%) 46%,
      rgb(0 0 0 / 22%) 64%,
      rgb(0 0 0 / 0%) 78%
    );
    pointer-events: none;
    position: absolute;
    width: var(--sky-size);
    /* Behind the page's content, above the page's background. Positioned and
       in-flow siblings would otherwise paint in source order, and the sky would
       wash over whatever it reaches - here, the top of the card. */
    z-index: -1;
  }

  :global(:root[data-theme='dark']) .night-sky {
    /* Dark: the same navy the halo is drawn on, so the logo reads as a window
       cut into a sky that carries on past it. */
    --sky-core: rgb(9 21 43 / 92%);
    --sky-glow: rgb(112 78 196 / 26%);
    --sky-star-bright: rgb(255 255 255 / 95%);
    --sky-star: rgb(223 234 255 / 72%);
    --sky-dust: rgb(186 206 255 / 46%);
  }

  .night-sky > span {
    border-radius: 50%;
    display: block;
    inset: 0;
    position: absolute;
  }

  .sky-wash {
    background-image:
      radial-gradient(circle at 38% 34%, var(--sky-glow) 0%, transparent 58%),
      radial-gradient(circle at 66% 62%, var(--sky-glow) 0%, transparent 52%),
      radial-gradient(circle at 50% 50%, var(--sky-core) 0%, transparent 68%);
  }

  /* Two star fields at different tile sizes, so the near one can drift in
     brightness against a steadier far one. */
  .sky-far {
    animation: sky-twinkle 11s ease-in-out 1.5s infinite;
    background-image:
      radial-gradient(circle at 12% 71%, var(--sky-dust) 0 0.8px, transparent 0.9px),
      radial-gradient(circle at 63% 24%, var(--sky-dust) 0 0.7px, transparent 0.8px),
      radial-gradient(circle at 88% 58%, var(--sky-dust) 0 0.8px, transparent 0.9px),
      radial-gradient(circle at 34% 12%, var(--sky-dust) 0 0.6px, transparent 0.7px),
      radial-gradient(circle at 47% 88%, var(--sky-star) 0 0.9px, transparent 1px);
    background-size:
      127px 101px,
      127px 101px,
      127px 101px,
      127px 101px,
      181px 149px;
  }

  .sky-near {
    animation: sky-twinkle 7s ease-in-out infinite;
    background-image:
      radial-gradient(circle at 21% 29%, var(--sky-star-bright) 0 1.3px, transparent 1.45px),
      radial-gradient(circle at 74% 67%, var(--sky-star-bright) 0 1.1px, transparent 1.25px),
      radial-gradient(circle at 8% 52%, var(--sky-star) 0 1px, transparent 1.15px),
      radial-gradient(circle at 55% 15%, var(--sky-star) 0 0.95px, transparent 1.1px),
      radial-gradient(circle at 91% 19%, var(--sky-star) 0 1px, transparent 1.15px);
    background-size:
      313px 271px,
      313px 271px,
      211px 173px,
      211px 173px,
      211px 173px;
  }

  /* Rests at full: reduced motion cuts every animation to one 0.01ms pass, which
     lands on the last keyframe, so the last keyframe has to be the resting one. */
  @keyframes sky-twinkle {
    0% {
      opacity: 1;
    }

    50% {
      opacity: 0.62;
    }

    100% {
      opacity: 1;
    }
  }
</style>

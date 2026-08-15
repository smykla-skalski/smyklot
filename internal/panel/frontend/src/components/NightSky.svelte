<script lang="ts">
  /**
   * A patch of night sky that opens out from a point and fades into the page.
   *
   * Night in both themes - it is space, not a tint of the page - so the fade is
   * what carries it onto a light canvas rather than a change of palette. Drawn
   * entirely from gradients: no raster to load, nothing to resample, and it
   * scales with whatever it is given.
   *
   * Anything laid over it needs light ink in both themes.
   */
  const {
    width = '210vw',
    height = '300vh',
  }: {
    /** How far the sky reaches across. Larger than the window is the point: the
     * fade then happens past the edges instead of inside them. */
    width?: string;
    /** How far it reaches up and down, for the same reason. */
    height?: string;
  } = $props();
</script>

<span class="night-sky" style="--sky-width: {width}; --sky-height: {height}" aria-hidden="true">
  <span class="sky-deep"></span>
  <span class="sky-nebula"></span>
  <span class="sky-dust"></span>
  <span class="sky-mid"></span>
  <span class="sky-bright"></span>
  <span class="sky-coloured"></span>
</span>

<style>
  .night-sky {
    /* The middle is the colour the halo is drawn on - `#09152B`, straight out of
       `smyklot-halo.svg` - so the sky reads as the emblem's own interior carrying
       on past the ring rather than a different dark placed behind it. */
    --sky-core: rgb(9 21 43);
    --sky-void: rgb(4 9 22);
    --sky-deep: rgb(8 16 38);
    --sky-violet: rgb(118 74 206 / 40%);
    --sky-teal: rgb(30 142 184 / 26%);
    --sky-rose: rgb(184 84 150 / 18%);

    /* Each star is a core, a bloom and a haze that reaches zero on its own hue -
       fading to `transparent` would pass through grey and dirty the glow. */
    --white: rgb(255 255 255);
    --white-bloom: rgb(214 230 255 / 62%);
    --white-haze: rgb(190 214 255 / 0%);
    --ice: rgb(196 224 255);
    --ice-bloom: rgb(148 194 255 / 58%);
    --ice-haze: rgb(120 176 255 / 0%);
    --amber: rgb(255 226 170);
    --amber-bloom: rgb(255 198 118 / 55%);
    --amber-haze: rgb(255 186 96 / 0%);
    --rose: rgb(255 200 214);
    --rose-bloom: rgb(255 152 186 / 50%);
    --rose-haze: rgb(255 140 178 / 0%);
    --dust: rgb(206 224 255 / 62%);
    --dust-haze: rgb(180 206 255 / 0%);

    display: block;
    height: var(--sky-height);
    /* The sky is larger than the window on purpose, so the fade happens off the
       edges rather than inside them: a ramp short enough to finish on screen is
       the thing that reads as a cut. Fourteen stops rather than four, and no
       straight run at the end, so the falloff has no step in it anywhere and no
       star is ever chopped by a boundary - they dim out with the sky around them.
       The radii are given, not left to `farthest-corner`, which sizes the ellipse
       to the corner and leaves the ramp unfinished on the short axis. */
    mask-image: radial-gradient(
      ellipse 50% 50% at 50% 50%,
      rgb(0 0 0 / 100%) 0%,
      rgb(0 0 0 / 100%) 30%,
      rgb(0 0 0 / 98%) 38%,
      rgb(0 0 0 / 95%) 45%,
      rgb(0 0 0 / 90%) 51%,
      rgb(0 0 0 / 82%) 57%,
      rgb(0 0 0 / 72%) 62%,
      rgb(0 0 0 / 60%) 67%,
      rgb(0 0 0 / 47%) 72%,
      rgb(0 0 0 / 34%) 77%,
      rgb(0 0 0 / 23%) 82%,
      rgb(0 0 0 / 13%) 87%,
      rgb(0 0 0 / 6%) 92%,
      rgb(0 0 0 / 2%) 96%,
      rgb(0 0 0 / 0%) 100%
    );
    pointer-events: none;
    position: absolute;
    width: var(--sky-width);
    /* Behind the page's content, above the page's background. Positioned and
       in-flow siblings would otherwise paint in source order, and the sky would
       wash over whatever it reaches. */
    z-index: -1;
  }

  .night-sky > span {
    display: block;
    inset: 0;
    position: absolute;
  }

  .sky-deep {
    background-image: radial-gradient(
      ellipse 50% 50% at 50% 50%,
      var(--sky-core) 0%,
      var(--sky-core) 12%,
      var(--sky-deep) 30%,
      var(--sky-void) 62%,
      var(--sky-void) 100%
    );
  }

  /* Clouds, off-centre and overlapping, so the sky has some weather in it rather
     than one even wash. */
  .sky-nebula {
    background-image:
      radial-gradient(ellipse 44% 58% at 31% 36%, var(--sky-violet) 0%, transparent 70%),
      radial-gradient(ellipse 38% 50% at 71% 62%, var(--sky-teal) 0%, transparent 66%),
      radial-gradient(ellipse 28% 40% at 57% 22%, var(--sky-violet) 0%, transparent 62%),
      radial-gradient(ellipse 26% 34% at 22% 74%, var(--sky-rose) 0%, transparent 60%);
  }

  /*
   * Stars are tiled rather than one gradient each, so a handful of rules draw
   * hundreds of them. Four layers at different tile sizes and different star
   * sizes, brightnesses and hues; the tile edges share no common factor, so the
   * repeat has no beat you can pick out. Every star sits well inside its tile,
   * bloom included, so none is clipped by a tile boundary.
   */
  .sky-dust {
    animation: sky-twinkle 13s ease-in-out 2.5s infinite;
    background-image:
      radial-gradient(circle at 17% 63%, var(--dust) 0 0.5px, var(--dust-haze) 2px),
      radial-gradient(circle at 58% 21%, var(--dust) 0 0.45px, var(--dust-haze) 1.8px),
      radial-gradient(circle at 81% 74%, var(--dust) 0 0.5px, var(--dust-haze) 2px),
      radial-gradient(circle at 36% 88%, var(--dust) 0 0.4px, var(--dust-haze) 1.6px),
      radial-gradient(circle at 69% 47%, var(--dust) 0 0.45px, var(--dust-haze) 1.8px),
      radial-gradient(circle at 27% 33%, var(--dust) 0 0.4px, var(--dust-haze) 1.6px);
    background-size: 89px 71px;
  }

  .sky-mid {
    animation: sky-twinkle 11s ease-in-out 1.5s infinite;
    background-image:
      radial-gradient(
        circle at 23% 68%,
        var(--white) 0 0.7px,
        var(--white-bloom) 1.5px,
        var(--white-haze) 4px
      ),
      radial-gradient(
        circle at 64% 26%,
        var(--ice) 0 0.65px,
        var(--ice-bloom) 1.4px,
        var(--ice-haze) 3.6px
      ),
      radial-gradient(
        circle at 86% 59%,
        var(--white) 0 0.6px,
        var(--white-bloom) 1.3px,
        var(--white-haze) 3.4px
      ),
      radial-gradient(
        circle at 41% 14%,
        var(--ice) 0 0.7px,
        var(--ice-bloom) 1.5px,
        var(--ice-haze) 3.8px
      ),
      radial-gradient(
        circle at 12% 39%,
        var(--white) 0 0.55px,
        var(--white-bloom) 1.2px,
        var(--white-haze) 3.2px
      );
    background-size: 173px 149px;
  }

  .sky-bright {
    animation: sky-twinkle 7s ease-in-out infinite;
    background-image:
      radial-gradient(
        circle at 26% 31%,
        var(--white) 0 1.3px,
        var(--white-bloom) 2.8px,
        var(--white-haze) 9px
      ),
      radial-gradient(
        circle at 73% 66%,
        var(--white) 0 1.1px,
        var(--white-bloom) 2.4px,
        var(--white-haze) 8px
      ),
      radial-gradient(
        circle at 52% 84%,
        var(--ice) 0 1px,
        var(--ice-bloom) 2.2px,
        var(--ice-haze) 7px
      );
    background-size: 311px 269px;
  }

  /* The few coloured ones, sparse enough to read as individuals. */
  .sky-coloured {
    animation: sky-twinkle 17s ease-in-out 4s infinite;
    background-image:
      radial-gradient(
        circle at 33% 24%,
        var(--amber) 0 1.2px,
        var(--amber-bloom) 2.6px,
        var(--amber-haze) 9px
      ),
      radial-gradient(
        circle at 78% 71%,
        var(--rose) 0 1.05px,
        var(--rose-bloom) 2.3px,
        var(--rose-haze) 8px
      ),
      radial-gradient(
        circle at 61% 39%,
        var(--amber) 0 0.85px,
        var(--amber-bloom) 1.9px,
        var(--amber-haze) 6px
      );
    background-size: 521px 443px;
  }

  /* Rests at full: reduced motion cuts every animation to one 0.01ms pass, which
     lands on the last keyframe, so the last keyframe has to be the resting one. */
  @keyframes sky-twinkle {
    0% {
      opacity: 1;
    }

    50% {
      opacity: 0.68;
    }

    100% {
      opacity: 1;
    }
  }
</style>

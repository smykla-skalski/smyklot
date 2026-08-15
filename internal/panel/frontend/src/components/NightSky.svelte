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
    width = '100vw',
    height = 'clamp(44rem, 480%, 72rem)',
  }: {
    /** How far the sky reaches across, before the mask fades it out. */
    width?: string;
    /**
     * How far it reaches up and down. A percentage measures against whatever it
     * is placed in, which is how it can follow a gap that changes with the
     * content instead of guessing at one. It is clamped at both ends: left to
     * grow it would run past the foot of the page when a short card leaves a
     * large gap, and left to shrink it would flatten into a band on a short
     * window, which is the shape that reads as squashed.
     */
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
    --sky-void: rgb(3 7 18);
    --sky-deep: rgb(6 13 30);
    --sky-violet: rgb(118 74 206 / 34%);
    --sky-teal: rgb(30 142 184 / 22%);
    --sky-rose: rgb(184 84 150 / 16%);

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
    /*
     * The boundary is one long tilted fade, cut off at the bottom by a straight
     * one, with banks of fog added back along it.
     *
     * The tilt runs down-and-left, so the sky gives out earlier on the left of
     * the window than on the right - the edge sits high on one side and carries
     * on down past the card on the other, rather than mirroring itself. A pair of
     * opposite tilts would be symmetric by construction; one is not.
     *
     * The straight fade underneath it exists for one reason: it reaches nothing
     * exactly at the box's own bottom edge. Left to the tilt alone that edge
     * still has substance in it where it crosses the screen, and an element
     * boundary with substance in it is a cut ruled across the page.
     *
     * A radial falloff was the obvious first try and cannot do any of this. Any
     * horizontal component to it dims the top corners more than the top middle -
     * the lens vignette - and pushing its radii past the box to avoid that leaves
     * the ramp unfinished at the bottom, which is the same cut again.
     *
     * The ramps are deliberately long, half the box and more. A short ramp reads
     * as an edge however smooth its stops are, and a long one is also what keeps
     * stars from being chopped: they dim out with the sky around them.
     */
    mask-image:
      /* Fog: soft banks sitting along the boundary, added to it rather than
         multiplied into it, so the sky bulges past the ramp in some places and
         not others. A clean line is the thing that reads as an edge. */
      /* Every bank has to reach nothing well inside the box. One centred at 80%
         with a 22% radius reached past the bottom edge, and fog that is still
         solid where the element stops is a cut with a soft top to it. */
      radial-gradient(ellipse 32% 14% at 15% 56%, rgb(0 0 0 / 82%), rgb(0 0 0 / 0%)),
      radial-gradient(ellipse 28% 13% at 42% 64%, rgb(0 0 0 / 78%), rgb(0 0 0 / 0%)),
      radial-gradient(ellipse 34% 15% at 76% 68%, rgb(0 0 0 / 84%), rgb(0 0 0 / 0%)),
      radial-gradient(ellipse 22% 11% at 60% 58%, rgb(0 0 0 / 72%), rgb(0 0 0 / 0%)),
      radial-gradient(ellipse 24% 12% at 29% 50%, rgb(0 0 0 / 74%), rgb(0 0 0 / 0%)),
      linear-gradient(
        197deg,
        rgb(0 0 0 / 100%) 0%,
        rgb(0 0 0 / 100%) 45%,
        rgb(0 0 0 / 97%) 50%,
        rgb(0 0 0 / 93%) 55%,
        rgb(0 0 0 / 87%) 60%,
        rgb(0 0 0 / 79%) 64%,
        rgb(0 0 0 / 69%) 68%,
        rgb(0 0 0 / 58%) 72%,
        rgb(0 0 0 / 46%) 76%,
        rgb(0 0 0 / 34%) 80%,
        rgb(0 0 0 / 23%) 84%,
        rgb(0 0 0 / 14%) 88%,
        rgb(0 0 0 / 7%) 92%,
        rgb(0 0 0 / 2%) 95%,
        rgb(0 0 0 / 0%) 98%
      ),
      linear-gradient(
        to bottom,
        rgb(0 0 0 / 100%) 0%,
        rgb(0 0 0 / 100%) 48%,
        rgb(0 0 0 / 96%) 54%,
        rgb(0 0 0 / 90%) 60%,
        rgb(0 0 0 / 81%) 66%,
        rgb(0 0 0 / 70%) 71%,
        rgb(0 0 0 / 57%) 76%,
        rgb(0 0 0 / 44%) 81%,
        rgb(0 0 0 / 31%) 86%,
        rgb(0 0 0 / 19%) 90%,
        rgb(0 0 0 / 10%) 94%,
        rgb(0 0 0 / 3%) 97%,
        rgb(0 0 0 / 0%) 100%
      );
    /* Read from the bottom up: the tilt meets the straight fade first, then each
       bank of fog is added onto what they left. */
    mask-composite: add, add, add, add, add, intersect, add;
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
     than one even wash. Held off the middle by a mask of their own: a cloud over
     the mark lifts the very colour the sky is supposed to start on. */
  .sky-nebula {
    background-image:
      radial-gradient(ellipse 40% 52% at 24% 32%, var(--sky-violet) 0%, transparent 70%),
      radial-gradient(ellipse 34% 44% at 78% 66%, var(--sky-teal) 0%, transparent 66%),
      radial-gradient(ellipse 26% 36% at 66% 18%, var(--sky-violet) 0%, transparent 62%),
      radial-gradient(ellipse 24% 32% at 16% 78%, var(--sky-rose) 0%, transparent 60%);
    mask-image: radial-gradient(
      ellipse 50% 50% at 50% 50%,
      rgb(0 0 0 / 0%) 0%,
      rgb(0 0 0 / 0%) 9%,
      rgb(0 0 0 / 55%) 18%,
      rgb(0 0 0 / 100%) 30%
    );
  }

  /*
   * Stars are tiled rather than one gradient each, so a handful of rules draw
   * hundreds of them. Four layers at different tile sizes and different star
   * sizes, brightnesses and hues; the tile edges share no common factor, so the
   * repeat has no beat you can pick out. Every star sits well inside its tile,
   * bloom included, so none is clipped by a tile boundary.
   *
   * An even scatter of them reads as wallpaper, though - a real sky is lumpy. So
   * each layer carries a coarse mask of its own at a scale far larger than its
   * tile, thinning it here and letting it through there. The layers clump in
   * different places, which is what turns a regular grid into drifts and gaps.
   */
  .sky-dust,
  .sky-mid,
  .sky-bright,
  .sky-coloured {
    mask-composite: intersect;
    mask-size: 100% 100%;
  }

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
    mask-image:
      radial-gradient(ellipse 38% 44% at 22% 28%, rgb(0 0 0 / 100%) 0%, rgb(0 0 0 / 15%) 100%),
      radial-gradient(ellipse 46% 40% at 74% 70%, rgb(0 0 0 / 100%) 0%, rgb(0 0 0 / 25%) 100%);
    mask-composite: add;
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
    mask-image:
      radial-gradient(ellipse 44% 38% at 68% 24%, rgb(0 0 0 / 100%) 0%, rgb(0 0 0 / 18%) 100%),
      radial-gradient(ellipse 36% 46% at 18% 66%, rgb(0 0 0 / 100%) 0%, rgb(0 0 0 / 22%) 100%);
    mask-composite: add;
  }

  /* The near ones, and the size range is deliberately wide: a field where every
     star is the same magnitude has no depth in it. */
  .sky-bright {
    animation: sky-twinkle 7s ease-in-out infinite;
    background-image:
      radial-gradient(
        circle at 26% 31%,
        var(--white) 0 1.9px,
        var(--white-bloom) 4.2px,
        var(--white-haze) 15px
      ),
      radial-gradient(
        circle at 73% 66%,
        var(--white) 0 1.1px,
        var(--white-bloom) 2.4px,
        var(--white-haze) 8px
      ),
      radial-gradient(
        circle at 52% 84%,
        var(--ice) 0 1.5px,
        var(--ice-bloom) 3.4px,
        var(--ice-haze) 12px
      ),
      radial-gradient(
        circle at 11% 72%,
        var(--white) 0 0.9px,
        var(--white-bloom) 2px,
        var(--white-haze) 6px
      );
    background-size: 311px 269px;
    mask-image: radial-gradient(
      ellipse 54% 48% at 38% 58%,
      rgb(0 0 0 / 100%) 0%,
      rgb(0 0 0 / 30%) 100%
    );
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

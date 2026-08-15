<script lang="ts">
  import type { CrossingEdge } from '../lib/crossing';
  import { rollGalaxies } from '../lib/galaxies';
  import { rollPulsars } from '../lib/pulsars';
  import { SkySlots } from '../lib/sky-slots';
  import NightAstronaut from './NightAstronaut.svelte';
  import NightMeteors from './NightMeteors.svelte';
  import NightRocket from './NightRocket.svelte';

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
  let {
    width = '100vw',
    height = 'clamp(50rem, 560%, 82rem)',
    rocket = true,
    rocketSpeed = 70,
    rocketTrailLife = 7,
    astronaut = true,
    meteors = true,
    slots = new SkySlots(),
    skyElement = $bindable(null),
  }: {
    /** How far the sky reaches across, before the mask fades it out. */
    width?: string;
    /**
     * How far it reaches up and down. A percentage measures against whatever it
     * is placed in, which is how it can follow a gap that changes with the
     * content instead of guessing at one. It is clamped at both ends: left to
     * grow it would run past the foot of the page when a short card leaves a
     * large gap, and left to shrink it would flatten into a band on a short
     * window, which is the shape that reads as squashed. The default was
     * raised once already - the night is welcome to reach lower - and the
     * ceiling is what keeps the fade finishing above the footer.
     */
    height?: string;
    /**
     * The easter egg: a little rocket wandering the sky. The flags gate
     * activity, never existence - flipping one off retires its flight
     * gracefully (the rocket departs, crossings finish off screen) and
     * flipping it back on lets the next flight come, so a theme switch
     * never cuts an animation mid-air.
     */
    rocket?: boolean;
    /** Its top speed, in CSS pixels per second; the flight varies its pace
     * beneath it and never passes it. */
    rocketSpeed?: number;
    /** Seconds each dash of its trail stays on the sky before dissolving. */
    rocketTrailLife?: number;
    /** The rarer easter egg: an astronaut adrift, tumbling across the sky. */
    astronaut?: boolean;
    /** The third: the odd small shower of meteors. */
    meteors?: boolean;
    /**
     * The seat budget capping how many easter eggs fly at once - two. Pass
     * the page's own instance when another flight home exists (the dark
     * theme's overlay), so the cap holds across both.
     */
    slots?: SkySlots;
    /**
     * The sky's own element, bindable out so the page can hand it to
     * overlay flights as the region that stays night on the light page.
     */
    skyElement?: HTMLElement | null;
  } = $props();

  /* No bottom entries or exits: the band's bottom edge lies mid-page, and a
     flight may only appear and disappear off screen. The other three edges
     of the band all lie past the window. */
  const BAND_EDGES: CrossingEdge[] = ['left', 'right', 'top'];

  /* Dealt once per mount: most skies carry no galaxy, and a sky that does
     carries it for the whole visit. The pulsars always come - a handful of
     stars beating to their own rhythms against the layers' even breath. */
  const galaxies = rollGalaxies(Math.random);
  const pulsars = rollPulsars(Math.random);
</script>

<!-- Sized through `style:` rather than a `style` attribute: the panel serves
     `style-src 'self'`, which drops a parsed style attribute, and the sky has no
     dimensions of its own without these two. See the note in app.css. -->
<span
  bind:this={skyElement}
  class="night-sky"
  style:--sky-width={width}
  style:--sky-height={height}
  aria-hidden="true"
>
  <span class="sky-deep"></span>
  <span class="sky-nebula"></span>
  {#each galaxies as galaxy (galaxy)}
    <span
      class={['sky-galaxy', galaxy.warm ? 'warm' : 'cool']}
      style:--galaxy-x="{galaxy.x}%"
      style:--galaxy-y="{galaxy.y}%"
      style:--galaxy-size="{galaxy.size}px"
      style:--galaxy-tilt="{galaxy.tilt}deg"
      style:--galaxy-glow={galaxy.glow}
    ></span>
  {/each}
  <span class="sky-dust"></span>
  <span class="sky-mid"></span>
  <span class="sky-bright"></span>
  <span class="sky-coloured"></span>
  {#each pulsars as pulsar (pulsar)}
    <span
      class={['sky-pulsar', pulsar.hue]}
      style:--pulsar-x="{pulsar.x}%"
      style:--pulsar-y="{pulsar.y}%"
      style:--pulsar-size="{pulsar.size}px"
      style:--pulse-duration="{pulsar.duration}s"
      style:--pulse-phase="-{pulsar.phase}s"
      style:--pulse-floor={pulsar.floor}
    ></span>
  {/each}
  <span class="sky-flight">
    <NightRocket
      speed={rocketSpeed}
      trailLife={rocketTrailLife}
      active={rocket}
      edges={BAND_EDGES}
      {slots}
    />
    <NightAstronaut active={astronaut} edges={BAND_EDGES} {slots} />
    <NightMeteors active={meteors} edges={BAND_EDGES} {slots} />
  </span>
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

  /* The easter eggs fly above the stars but inside the sky's mask, so they
     fade out with the sky rather than crossing the page - and the sky's own
     stacking keeps them behind everything laid on top. The flight band stops
     short of the bottom: below that line the fade has taken most of the sky,
     and a rocket spending its life where it cannot be seen is no easter egg.
     The canvases lie on top of one another; stacked in flow, the second
     would sit below the band instead of over it. */
  .night-sky > .sky-flight {
    inset: 0 0 34% 0;
  }

  .sky-flight > :global(canvas) {
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

  /* A galaxy, when the mount dealt one: gradients stacked into an inclined
     disc - a core, the disc through it, a wide dim halo, and two arm
     smudges set off either side of the major axis so the shape is not a
     perfect lens. Every glow ends on its own hue at nothing, like the
     stars' - through `transparent` it would grey on the way out. Steady,
     between the clouds and the stars: galaxies do not twinkle, and the
     nearer stars drift over them. */
  .night-sky > .sky-galaxy {
    background-image:
      radial-gradient(
        ellipse 15% 20% at 50% 50%,
        var(--galaxy-core) 0%,
        var(--galaxy-core-end) 70%
      ),
      radial-gradient(ellipse 21% 13% at 35% 42%, var(--galaxy-arm) 0%, var(--galaxy-arm-end) 72%),
      radial-gradient(ellipse 21% 13% at 65% 58%, var(--galaxy-arm) 0%, var(--galaxy-arm-end) 72%),
      radial-gradient(
        ellipse 44% 16% at 50% 50%,
        var(--galaxy-disc) 0%,
        var(--galaxy-disc-end) 74%
      ),
      radial-gradient(ellipse 50% 30% at 50% 50%, var(--galaxy-halo) 0%, var(--galaxy-halo-end) 80%);
    height: calc(var(--galaxy-size) * 0.62);
    inset: auto;
    left: var(--galaxy-x);
    opacity: var(--galaxy-glow);
    rotate: var(--galaxy-tilt);
    top: var(--galaxy-y);
    translate: -50% -50%;
    width: var(--galaxy-size);
  }

  .sky-galaxy.cool {
    --galaxy-core: rgb(244 240 255 / 78%);
    --galaxy-core-end: rgb(244 240 255 / 0%);
    --galaxy-arm: rgb(168 190 255 / 24%);
    --galaxy-arm-end: rgb(168 190 255 / 0%);
    --galaxy-disc: rgb(190 208 255 / 30%);
    --galaxy-disc-end: rgb(190 208 255 / 0%);
    --galaxy-halo: rgb(140 150 240 / 15%);
    --galaxy-halo-end: rgb(140 150 240 / 0%);
  }

  .sky-galaxy.warm {
    --galaxy-core: rgb(255 240 216 / 78%);
    --galaxy-core-end: rgb(255 240 216 / 0%);
    --galaxy-arm: rgb(255 198 154 / 20%);
    --galaxy-arm-end: rgb(255 198 154 / 0%);
    --galaxy-disc: rgb(255 212 176 / 25%);
    --galaxy-disc-end: rgb(255 212 176 / 0%);
    --galaxy-halo: rgb(226 156 160 / 13%);
    --galaxy-halo-end: rgb(226 156 160 / 0%);
  }

  /*
   * Stars are tiled rather than one gradient each, so a page of rules draws
   * hundreds of them. Four layers at different tile sizes and different star
   * sizes, brightnesses and hues.
   *
   * The tiles are big - 227x193 up to 887x769, every dimension prime - and
   * that size is the lesson of the first version, whose tiles were a third
   * of this: a distinctive star returning every 311px is a pattern the eye
   * finds in seconds, however coprime the tile edges are. Now the brightest
   * layer repeats about twice across a wide window and the repeat has
   * nothing left to catch. Each layer is also phase-shifted, so no corner
   * of the element shows every tile's origin at once, and every star sits
   * far enough inside its tile that no bloom is clipped at a boundary.
   *
   * An even scatter still reads as wallpaper - a real sky is lumpy. So each
   * layer carries a coarse mask of its own at a scale far larger than its
   * tile, thinning it here and letting it through there. The layers clump in
   * different places, which is what turns a regular grid into drifts and
   * gaps. The dots below were dealt once from a seeded scatter and written
   * down; regenerating them is cosmetic, never required.
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
      radial-gradient(circle at 25.8% 60.3%, var(--dust) 0 0.38px, var(--dust-haze) 1.5px),
      radial-gradient(circle at 27.5% 43%, var(--dust) 0 0.49px, var(--dust-haze) 1.9px),
      radial-gradient(circle at 95.3% 23.9%, var(--dust) 0 0.43px, var(--dust-haze) 1.7px),
      radial-gradient(circle at 11.8% 65.3%, var(--dust) 0 0.51px, var(--dust-haze) 2.1px),
      radial-gradient(circle at 81.8% 62.4%, var(--dust) 0 0.44px, var(--dust-haze) 1.8px),
      radial-gradient(circle at 61.9% 81.6%, var(--dust) 0 0.43px, var(--dust-haze) 1.7px),
      radial-gradient(circle at 2.4% 17.9%, var(--dust) 0 0.51px, var(--dust-haze) 2px),
      radial-gradient(circle at 81.4% 56.7%, var(--dust) 0 0.41px, var(--dust-haze) 1.6px),
      radial-gradient(circle at 62.1% 7.1%, var(--dust) 0 0.46px, var(--dust-haze) 1.8px),
      radial-gradient(circle at 3.5% 17%, var(--dust) 0 0.49px, var(--dust-haze) 2px),
      radial-gradient(circle at 63.3% 50.1%, var(--dust) 0 0.39px, var(--dust-haze) 1.6px),
      radial-gradient(circle at 5.2% 31.6%, var(--dust) 0 0.5px, var(--dust-haze) 2px),
      radial-gradient(circle at 42.9% 58.8%, var(--dust) 0 0.54px, var(--dust-haze) 2.2px),
      radial-gradient(circle at 38% 41.4%, var(--dust) 0 0.46px, var(--dust-haze) 1.9px),
      radial-gradient(circle at 49.2% 95.3%, var(--dust) 0 0.49px, var(--dust-haze) 2px),
      radial-gradient(circle at 72.1% 58.1%, var(--dust) 0 0.44px, var(--dust-haze) 1.8px),
      radial-gradient(circle at 35.6% 89.2%, var(--dust) 0 0.4px, var(--dust-haze) 1.6px),
      radial-gradient(circle at 10% 63.9%, var(--dust) 0 0.41px, var(--dust-haze) 1.6px),
      radial-gradient(circle at 96.7% 88.4%, var(--dust) 0 0.52px, var(--dust-haze) 2.1px),
      radial-gradient(circle at 43.5% 26.6%, var(--dust) 0 0.51px, var(--dust-haze) 2px),
      radial-gradient(circle at 38% 6.3%, var(--dust) 0 0.38px, var(--dust-haze) 1.5px),
      radial-gradient(circle at 97.2% 36.9%, var(--dust) 0 0.42px, var(--dust-haze) 1.7px),
      radial-gradient(circle at 24% 92.3%, var(--dust) 0 0.39px, var(--dust-haze) 1.6px),
      radial-gradient(circle at 63.4% 16.4%, var(--dust) 0 0.42px, var(--dust-haze) 1.7px),
      radial-gradient(circle at 12.5% 44.7%, var(--dust) 0 0.55px, var(--dust-haze) 2.2px),
      radial-gradient(circle at 45.6% 44.5%, var(--dust) 0 0.51px, var(--dust-haze) 2px),
      radial-gradient(circle at 30.7% 17.8%, var(--dust) 0 0.41px, var(--dust-haze) 1.6px),
      radial-gradient(circle at 6.8% 56.6%, var(--dust) 0 0.38px, var(--dust-haze) 1.5px),
      radial-gradient(circle at 20.5% 2.5%, var(--dust) 0 0.42px, var(--dust-haze) 1.7px),
      radial-gradient(circle at 22.2% 28%, var(--dust) 0 0.51px, var(--dust-haze) 2px),
      radial-gradient(circle at 88.7% 66.5%, var(--dust) 0 0.53px, var(--dust-haze) 2.1px),
      radial-gradient(circle at 78.8% 47.1%, var(--dust) 0 0.43px, var(--dust-haze) 1.7px),
      radial-gradient(circle at 59.8% 60.8%, var(--dust) 0 0.38px, var(--dust-haze) 1.5px),
      radial-gradient(circle at 44.9% 66.9%, var(--dust) 0 0.51px, var(--dust-haze) 2px),
      radial-gradient(circle at 89.5% 94.2%, var(--dust) 0 0.53px, var(--dust-haze) 2.1px),
      radial-gradient(circle at 97% 55.6%, var(--dust) 0 0.4px, var(--dust-haze) 1.6px);
    background-size: 227px 193px;
    mask-image:
      radial-gradient(ellipse 38% 44% at 22% 28%, rgb(0 0 0 / 100%) 0%, rgb(0 0 0 / 15%) 100%),
      radial-gradient(ellipse 46% 40% at 74% 70%, rgb(0 0 0 / 100%) 0%, rgb(0 0 0 / 25%) 100%);
    mask-composite: add;
  }

  .sky-mid {
    animation: sky-twinkle 11s ease-in-out 1.5s infinite;
    background-image:
      radial-gradient(
        circle at 27.9% 46.2%,
        var(--white) 0 0.72px,
        var(--white-bloom) 1.5px,
        var(--white-haze) 4px
      ),
      radial-gradient(
        circle at 16.7% 68.5%,
        var(--white) 0 0.6px,
        var(--white-bloom) 1.3px,
        var(--white-haze) 3.3px
      ),
      radial-gradient(
        circle at 47% 32.2%,
        var(--white) 0 0.72px,
        var(--white-bloom) 1.6px,
        var(--white-haze) 4px
      ),
      radial-gradient(
        circle at 14.1% 43.7%,
        var(--white) 0 0.55px,
        var(--white-bloom) 1.2px,
        var(--white-haze) 3px
      ),
      radial-gradient(
        circle at 56.8% 62.3%,
        var(--white) 0 0.59px,
        var(--white-bloom) 1.3px,
        var(--white-haze) 3.2px
      ),
      radial-gradient(
        circle at 2.6% 85.1%,
        var(--white) 0 0.74px,
        var(--white-bloom) 1.6px,
        var(--white-haze) 4.1px
      ),
      radial-gradient(
        circle at 85.6% 53.6%,
        var(--white) 0 0.56px,
        var(--white-bloom) 1.2px,
        var(--white-haze) 3.1px
      ),
      radial-gradient(
        circle at 36.4% 38%,
        var(--ice) 0 0.66px,
        var(--ice-bloom) 1.4px,
        var(--ice-haze) 3.6px
      ),
      radial-gradient(
        circle at 63.9% 14.9%,
        var(--ice) 0 0.62px,
        var(--ice-bloom) 1.3px,
        var(--ice-haze) 3.4px
      ),
      radial-gradient(
        circle at 27% 83.4%,
        var(--white) 0 0.77px,
        var(--white-bloom) 1.7px,
        var(--white-haze) 4.2px
      ),
      radial-gradient(
        circle at 94.3% 89.5%,
        var(--white) 0 0.65px,
        var(--white-bloom) 1.4px,
        var(--white-haze) 3.6px
      ),
      radial-gradient(
        circle at 24.6% 31.6%,
        var(--white) 0 0.64px,
        var(--white-bloom) 1.4px,
        var(--white-haze) 3.5px
      ),
      radial-gradient(
        circle at 78.2% 43.4%,
        var(--white) 0 0.65px,
        var(--white-bloom) 1.4px,
        var(--white-haze) 3.6px
      ),
      radial-gradient(
        circle at 90.2% 26.5%,
        var(--white) 0 0.71px,
        var(--white-bloom) 1.5px,
        var(--white-haze) 3.9px
      ),
      radial-gradient(
        circle at 84.6% 97.8%,
        var(--white) 0 0.65px,
        var(--white-bloom) 1.4px,
        var(--white-haze) 3.6px
      ),
      radial-gradient(
        circle at 34.6% 63.7%,
        var(--white) 0 0.74px,
        var(--white-bloom) 1.6px,
        var(--white-haze) 4.1px
      ),
      radial-gradient(
        circle at 96% 65.9%,
        var(--ice) 0 0.67px,
        var(--ice-bloom) 1.4px,
        var(--ice-haze) 3.7px
      ),
      radial-gradient(
        circle at 59.7% 88.6%,
        var(--ice) 0 0.69px,
        var(--ice-bloom) 1.5px,
        var(--ice-haze) 3.8px
      ),
      radial-gradient(
        circle at 28.4% 41.1%,
        var(--white) 0 0.67px,
        var(--white-bloom) 1.4px,
        var(--white-haze) 3.7px
      ),
      radial-gradient(
        circle at 54.3% 3%,
        var(--white) 0 0.75px,
        var(--white-bloom) 1.6px,
        var(--white-haze) 4.1px
      ),
      radial-gradient(
        circle at 58.8% 81.5%,
        var(--ice) 0 0.64px,
        var(--ice-bloom) 1.4px,
        var(--ice-haze) 3.5px
      ),
      radial-gradient(
        circle at 59.8% 54%,
        var(--ice) 0 0.76px,
        var(--ice-bloom) 1.6px,
        var(--ice-haze) 4.2px
      );
    background-position: 89px 47px;
    background-size: 397px 337px;
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
        circle at 82.6% 25.2%,
        var(--white) 0 2.01px,
        var(--white-bloom) 4.4px,
        var(--white-haze) 15.1px
      ),
      radial-gradient(
        circle at 89.2% 89.6%,
        var(--ice) 0 1.56px,
        var(--ice-bloom) 3.4px,
        var(--ice-haze) 10.1px
      ),
      radial-gradient(
        circle at 48% 70.6%,
        var(--white) 0 1.55px,
        var(--white-bloom) 3.4px,
        var(--white-haze) 10.1px
      ),
      radial-gradient(
        circle at 45.4% 90.3%,
        var(--white) 0 1.55px,
        var(--white-bloom) 3.4px,
        var(--white-haze) 10px
      ),
      radial-gradient(
        circle at 76.4% 86.1%,
        var(--white) 0 1.22px,
        var(--white-bloom) 2.7px,
        var(--white-haze) 7.9px
      ),
      radial-gradient(
        circle at 41.5% 64%,
        var(--ice) 0 0.87px,
        var(--ice-bloom) 1.9px,
        var(--ice-haze) 5.7px
      ),
      radial-gradient(
        circle at 95.1% 57.6%,
        var(--white) 0 0.86px,
        var(--white-bloom) 1.9px,
        var(--white-haze) 5.6px
      ),
      radial-gradient(
        circle at 92.3% 44.7%,
        var(--white) 0 1.01px,
        var(--white-bloom) 2.2px,
        var(--white-haze) 6.6px
      ),
      radial-gradient(
        circle at 20.6% 12.4%,
        var(--white) 0 1.1px,
        var(--white-bloom) 2.4px,
        var(--white-haze) 7.2px
      ),
      radial-gradient(
        circle at 8.5% 56.4%,
        var(--white) 0 1.02px,
        var(--white-bloom) 2.2px,
        var(--white-haze) 6.6px
      ),
      radial-gradient(
        circle at 45.9% 9.9%,
        var(--ice) 0 1.21px,
        var(--ice-bloom) 2.7px,
        var(--ice-haze) 7.9px
      ),
      radial-gradient(
        circle at 66.5% 75.7%,
        var(--white) 0 1.18px,
        var(--white-bloom) 2.6px,
        var(--white-haze) 7.7px
      ),
      radial-gradient(
        circle at 82.2% 37.7%,
        var(--ice) 0 1.26px,
        var(--ice-bloom) 2.8px,
        var(--ice-haze) 8.2px
      ),
      radial-gradient(
        circle at 10.9% 79.3%,
        var(--ice) 0 1.04px,
        var(--ice-bloom) 2.3px,
        var(--ice-haze) 6.7px
      ),
      radial-gradient(
        circle at 21.7% 80.4%,
        var(--ice) 0 0.86px,
        var(--ice-bloom) 1.9px,
        var(--ice-haze) 5.6px
      ),
      radial-gradient(
        circle at 52.6% 84.2%,
        var(--ice) 0 1.27px,
        var(--ice-bloom) 2.8px,
        var(--ice-haze) 8.2px
      );
    background-position: 211px 131px;
    background-size: 683px 587px;
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
        circle at 89.3% 5.9%,
        var(--amber) 0 1.02px,
        var(--amber-bloom) 2.2px,
        var(--amber-haze) 7.1px
      ),
      radial-gradient(
        circle at 21.3% 49.3%,
        var(--rose) 0 1.26px,
        var(--rose-bloom) 2.8px,
        var(--rose-haze) 8.8px
      ),
      radial-gradient(
        circle at 71.5% 39.1%,
        var(--amber) 0 1.08px,
        var(--amber-bloom) 2.4px,
        var(--amber-haze) 7.6px
      ),
      radial-gradient(
        circle at 12.3% 60.6%,
        var(--rose) 0 1.07px,
        var(--rose-bloom) 2.3px,
        var(--rose-haze) 7.5px
      ),
      radial-gradient(
        circle at 3.4% 55.2%,
        var(--amber) 0 1.18px,
        var(--amber-bloom) 2.6px,
        var(--amber-haze) 8.3px
      ),
      radial-gradient(
        circle at 20.8% 34.5%,
        var(--rose) 0 0.96px,
        var(--rose-bloom) 2.1px,
        var(--rose-haze) 6.7px
      ),
      radial-gradient(
        circle at 6.3% 12.6%,
        var(--amber) 0 1.16px,
        var(--amber-bloom) 2.6px,
        var(--amber-haze) 8.1px
      ),
      radial-gradient(
        circle at 78.9% 72.1%,
        var(--rose) 0 1px,
        var(--rose-bloom) 2.2px,
        var(--rose-haze) 7px
      );
    background-position: 337px 211px;
    background-size: 887px 769px;
  }

  /* The pulsars: one star each, beating alone. The layers' twinkle moves
     whole sheets at once, and these are the exceptions that make that read
     as a sky rather than a fade - each has its own period, its own phase
     (a negative delay, so no two start together) and its own floor, some
     barely flickering, some nearly going out. Opacity only: the compositor
     carries it without repainting anything. */
  .night-sky > .sky-pulsar {
    animation: sky-pulse var(--pulse-duration) ease-in-out var(--pulse-phase) infinite;
    background-image: radial-gradient(
      circle at 50% 50%,
      var(--pulsar-core) 0 7%,
      var(--pulsar-bloom) 16%,
      var(--pulsar-haze) 50%
    );
    height: var(--pulsar-size);
    inset: auto;
    left: var(--pulsar-x);
    top: var(--pulsar-y);
    translate: -50% -50%;
    width: var(--pulsar-size);
  }

  .sky-pulsar.white {
    --pulsar-core: var(--white);
    --pulsar-bloom: var(--white-bloom);
    --pulsar-haze: var(--white-haze);
  }

  .sky-pulsar.ice {
    --pulsar-core: var(--ice);
    --pulsar-bloom: var(--ice-bloom);
    --pulsar-haze: var(--ice-haze);
  }

  .sky-pulsar.amber {
    --pulsar-core: var(--amber);
    --pulsar-bloom: var(--amber-bloom);
    --pulsar-haze: var(--amber-haze);
  }

  .sky-pulsar.rose {
    --pulsar-core: var(--rose);
    --pulsar-bloom: var(--rose-bloom);
    --pulsar-haze: var(--rose-haze);
  }

  /* Rests at full, like the twinkle: reduced motion squashes this to one
     0.01ms pass that lands on the last keyframe. */
  @keyframes sky-pulse {
    0% {
      opacity: 1;
    }

    50% {
      opacity: var(--pulse-floor);
    }

    100% {
      opacity: 1;
    }
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

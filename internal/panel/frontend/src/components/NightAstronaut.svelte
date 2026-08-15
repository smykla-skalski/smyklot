<script lang="ts">
  import type { Attachment } from 'svelte/attachments';

  import { Drift } from '../lib/astronaut';
  import type { CrossingEdge } from '../lib/crossing';
  import { fadeBlend, mixInk, type Ink } from '../lib/ink';
  import type { SkySlots } from '../lib/sky-slots';

  /**
   * The sky's other easter egg, rarer than the rocket: a small line-drawn
   * astronaut tumbling slowly around itself as it drifts in a straight line
   * from outside one edge of the canvas to outside another - sometimes
   * across, sometimes down, sometimes on a long diagonal, each crossing at
   * its own speed and its own rate of roll. No engine and no steering:
   * everything about a crossing is decided when it begins, which is what
   * makes it read as adrift rather than flown. What keeps it from reading
   * as dead is the small life left in it - limbs stirring slowly against
   * the tumble, and a severed tether rippling along behind.
   *
   * The easter eggs are independent - the rocket may well be flying while
   * the astronaut passes - but never a crowd: a crossing takes a seat from
   * the shared `SkySlots` budget and gives it back as it leaves. Each egg
   * keeps to its own canvas and its own loop, and this one sleeps far more
   * than it runs: between crossings there is no animation frame at all,
   * only one pending timer - which is also what wakes it again after its
   * home has been inactive through a theme switch.
   *
   * The same guarantees as the rocket's: the loop stops when the canvas is
   * off screen, nothing runs or shows under `prefers-reduced-motion` (the
   * CSS animation squash cannot reach a canvas, so the gate is here), and a
   * hidden tab suspends the crossing where it stands.
   */
  const {
    edges,
    active = true,
    slots = undefined,
    sky = null,
  }: {
    /**
     * The canvas edges a crossing may begin and end past. A crossing only
     * ever appears and disappears off screen, so a caller whose canvas has
     * an edge lying mid-page leaves that edge out.
     */
    edges?: CrossingEdge[];
    /**
     * Whether this home may start crossings. Flipping it off never cuts one
     * short - a crossing in flight finishes off screen; only new ones wait.
     */
    active?: boolean;
    /** The shared seat budget capping how many easter eggs fly at once. */
    slots?: SkySlots;
    /**
     * The element that stays night on the light page. A crossing finishing
     * after a dark-to-light switch darkens its ink smoothly below this
     * element's fade, so it stays visible to its off-screen end.
     */
    sky?: HTMLElement | null;
  } = $props();

  const TAU = Math.PI * 2;

  /* The same starlight ink as the rocket's hull - they are the same
     universe - with the same dark twin for a light ground. */
  const INK: Ink = [216, 232, 255];
  const INK_DARK: Ink = [40, 51, 74];

  /* The first sighting comes reasonably soon; later ones keep their
     distance. Retry is for a timer that fires while the canvas is off screen
     or unsized - the crossing waits rather than happening unseen. */
  const FIRST_MIN_S = 8;
  const FIRST_SPAN_S = 27;
  const NEXT_MIN_S = 25;
  const NEXT_SPAN_S = 65;
  const RETRY_MIN_S = 5;
  const RETRY_SPAN_S = 10;

  function drawAstronaut(
    ctx: CanvasRenderingContext2D,
    x: number,
    y: number,
    angle: number,
    time: number,
    ink: string,
  ): void {
    ctx.save();
    ctx.translate(x, y);
    ctx.rotate(angle);
    /* Scaled as one figure: about 27px head to toe - smaller than the
       rocket's 32px hull, but the strokes keep enough air between them to
       read as helmet, pack and limbs rather than a scribble. The line
       widths below are pre-scale, so the drawn lines stay hairline. */
    ctx.scale(1.35, 1.35);
    ctx.strokeStyle = ink;
    ctx.globalAlpha = 0.92;
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    /* The severed line, first, so the suit overlaps its root. It leaves the
       bottom of the pack, away from every limb, and ripples with a slow
       travelling wave that grows toward the free end - a cut tether
       floating, not a tail hanging. */
    ctx.lineWidth = 0.85;
    ctx.beginPath();
    ctx.moveTo(-4.6, 2.9);
    let wireX = 0;
    let wireY = 0;
    for (let i = 1; i <= 8; i += 1) {
      const s = i / 8;
      wireX = -4.6 - s * 11.5;
      wireY = 2.9 + s * 5.5 + Math.sin(s * 5.2 - time * 1.1) * 1.9 * s;
      ctx.lineTo(wireX, wireY);
    }
    ctx.stroke();
    /* The broken end: a short stub off at an angle, the plug that no longer
       plugs into anything. */
    ctx.beginPath();
    ctx.moveTo(wireX, wireY);
    ctx.lineTo(wireX - 0.9, wireY + 1.7);
    ctx.stroke();
    /* The suit. Every joint drifts on its own slow sine - amplitudes under a
       pixel, periods of many seconds - which is the difference between a
       figure adrift and a figure dead. */
    const armA = Math.sin(time * 0.7 + 1) * 0.9;
    const armB = Math.sin(time * 0.53 + 3) * 0.8;
    const legA = Math.sin(time * 0.61 + 2) * 0.9;
    const legB = Math.sin(time * 0.47) * 0.8;
    ctx.lineWidth = 1;
    ctx.beginPath();
    /* Torso: collar to hips, a rounded box under the helmet. */
    ctx.moveTo(-3.2, -1.6);
    ctx.lineTo(-2.8, 3.6);
    ctx.quadraticCurveTo(0, 4.8, 2.8, 3.6);
    ctx.lineTo(3.2, -1.6);
    ctx.lineTo(-3.2, -1.6);
    /* The life-support pack - the box on the back is what says astronaut
       from any distance. */
    ctx.moveTo(-3.2, -1.2);
    ctx.lineTo(-5.6, -1.2);
    ctx.lineTo(-5.6, 2.9);
    ctx.lineTo(-3.2, 2.9);
    /* Arms bent adrift: one reaching up and out, one raised behind, both
       clear of the pack and the line. */
    ctx.moveTo(3, -0.9);
    ctx.lineTo(6.4, -3.2 + armA);
    ctx.lineTo(8.2, -2 + armA * 1.6);
    ctx.moveTo(-3.2, -0.6);
    ctx.lineTo(-6.6, -2.8 + armB);
    ctx.lineTo(-8.2, -1.2 + armB * 1.7);
    /* Legs splayed wide, knees soft - nothing is braced against anything. */
    ctx.moveTo(-1.5, 4.3);
    ctx.lineTo(-3.9 - legA * 0.5, 7.2);
    ctx.lineTo(-3.3 - legA, 10);
    ctx.moveTo(1.5, 4.3);
    ctx.lineTo(4 + legB * 0.5, 6.8);
    ctx.lineTo(5.9 + legB, 9.2);
    ctx.stroke();
    /* Helmet over the collar, the visor turned a little to one side. */
    ctx.beginPath();
    ctx.arc(0, -5.8, 4.3, 0, TAU);
    ctx.stroke();
    ctx.beginPath();
    ctx.arc(1, -5.8, 2.4, -1, 1);
    ctx.stroke();
    ctx.restore();
    ctx.globalAlpha = 1;
  }

  function drifting(): Attachment<HTMLCanvasElement> {
    return (canvas) => {
      const ctx = canvas.getContext('2d');
      if (ctx === null) return;

      let drift: Drift | null = null;
      let timer = 0;
      let raf = 0;
      let running = false;
      let seat = false;
      let inView = true;
      let lastTs = 0;
      let simT = 0;
      let cssW = 0;
      let cssH = 0;

      const reduce = window.matchMedia('(prefers-reduced-motion: reduce)');

      /* The sky's fade in canvas coordinates, for the retiring-ink blend.
         Measured when a crossing starts and on resize - a crossing is long,
         but the layout under it is still. */
      let fadeStart = 0;
      let fadeEnd = 0;

      const syncFade = (): void => {
        if (sky === null) return;
        const own = canvas.getBoundingClientRect();
        const patch = sky.getBoundingClientRect();
        fadeStart = patch.top - own.top + patch.height * 0.42;
        fadeEnd = patch.top - own.top + patch.height * 0.8;
      };

      const schedule = (min: number, span: number): void => {
        clearTimeout(timer);
        timer = window.setTimeout(begin, (min + Math.random() * span) * 1000);
      };

      /* An idle canvas holds no bitmap - a backing store this size is tens
         of megabytes, and this component sleeps far more than it runs. The
         bitmap exists exactly as long as a crossing does. */
      const applyBitmap = (): void => {
        const dpr = Math.min(window.devicePixelRatio || 1, 2);
        canvas.width = Math.round(cssW * dpr);
        canvas.height = Math.round(cssH * dpr);
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      };

      const releaseBitmap = (): void => {
        canvas.width = 0;
        canvas.height = 0;
      };

      const begin = (): void => {
        if (!active || !inView || reduce.matches || cssW === 0 || cssH === 0) {
          schedule(RETRY_MIN_S, RETRY_SPAN_S);
          return;
        }
        /* The sky seats two; a full house costs this visitor one retry. */
        if (slots !== undefined && !slots.take()) {
          schedule(RETRY_MIN_S, RETRY_SPAN_S);
          return;
        }
        seat = true;
        applyBitmap();
        syncFade();
        drift = new Drift({ width: cssW, height: cssH, edges, random: Math.random });
        sync();
      };

      const releaseSeat = (): void => {
        if (!seat) return;
        seat = false;
        slots?.release();
      };

      const tick = (ts: number): void => {
        raf = requestAnimationFrame(tick);
        if (drift === null) return;
        /* Clamped like the rocket's: a hidden tab hands the whole absence
           back as one dt, and the crossing resumes instead of skipping. */
        const dt = Math.min(Math.max((ts - lastTs) / 1000, 0), 0.1);
        lastTs = ts;
        simT += dt;
        drift.step(dt);
        ctx.clearRect(0, 0, cssW, cssH);
        if (drift.done) {
          drift = null;
          releaseSeat();
          releaseBitmap();
          sync();
          schedule(NEXT_MIN_S, NEXT_SPAN_S);
          return;
        }
        /* Starlight while the home is active; a crossing finishing on the
           light page blends dark as it moves below the sky's fade. */
        const ground = !active && sky !== null ? fadeBlend(drift.y, fadeStart, fadeEnd) : 0;
        drawAstronaut(ctx, drift.x, drift.y, drift.angle, simT, mixInk(INK, INK_DARK, ground));
      };

      const sync = (): void => {
        const run = drift !== null && inView && !reduce.matches;
        if (run !== running) {
          running = run;
          if (run) {
            lastTs = performance.now();
            raf = requestAnimationFrame(tick);
          } else {
            cancelAnimationFrame(raf);
          }
        }
        if (reduce.matches && drift !== null) {
          /* Reduced motion mid-crossing: the visitor simply is not there. */
          drift = null;
          releaseSeat();
          releaseBitmap();
        }
      };

      const onReduce = (): void => {
        sync();
        clearTimeout(timer);
        /* Coming back from reduced motion starts the clock afresh; going
           into it leaves no timer ticking for nobody. */
        if (!reduce.matches) schedule(NEXT_MIN_S, NEXT_SPAN_S);
      };

      const resize = (): void => {
        const w = canvas.clientWidth;
        const h = canvas.clientHeight;
        if (w === 0 || h === 0) return;
        cssW = w;
        cssH = h;
        /* Only a crossing in flight re-sizes its bitmap; idle stays bare. */
        if (drift !== null) {
          applyBitmap();
          syncFade();
        }
      };

      const ro = new ResizeObserver(resize);
      ro.observe(canvas);

      const io = new IntersectionObserver((entries) => {
        for (const entry of entries) inView = entry.isIntersecting;
        sync();
      });
      io.observe(canvas);

      reduce.addEventListener('change', onReduce);
      schedule(FIRST_MIN_S, FIRST_SPAN_S);

      return () => {
        cancelAnimationFrame(raf);
        clearTimeout(timer);
        ro.disconnect();
        io.disconnect();
        reduce.removeEventListener('change', onReduce);
        releaseSeat();
      };
    };
  }
</script>

<canvas class="night-astronaut" aria-hidden="true" {@attach drifting()}></canvas>

<style>
  .night-astronaut {
    display: block;
    height: 100%;
    pointer-events: none;
    width: 100%;
  }
</style>

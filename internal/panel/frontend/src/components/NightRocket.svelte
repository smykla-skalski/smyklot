<script lang="ts">
  import type { Attachment } from 'svelte/attachments';

  import type { CrossingEdge } from '../lib/crossing';
  import { fadeBlend, mixInk, type Ink } from '../lib/ink';
  import { Flight, TrailEmitter, type TrailDash } from '../lib/rocket';
  import type { SkySlots } from '../lib/sky-slots';

  /**
   * The sky's easter egg: a small line-drawn rocket that wanders wherever
   * this canvas is placed, leaving a dashed trail that remembers the shape of
   * every curve it flew for a while and then dissolves. It cruises, banks,
   * flies the odd full circle or a sudden tight turn, and sometimes glides to
   * a stop, sits, turns on the spot and burns back up to speed. The behaviour
   * lives in `lib/rocket.ts`; this component only sizes the canvas and draws.
   *
   * The flight begins and ends off screen. A rocket enters by flying in from
   * past an allowed edge, and when its home goes inactive - a theme switch -
   * it is not cut: it departs at full burn through the nearest allowed edge,
   * its trail fades to nothing, and only then is the seat given back and the
   * canvas left bare. While inactive a slow timer keeps asking whether to
   * fly again, which is also what brings the rocket back when its home
   * reactivates - existence is never toggled, only activity.
   *
   * Placed inside `NightSky`, the sky's own mask fades the rocket out with
   * the stars, and the sky's stacking keeps it behind the page's content -
   * neither is re-implemented here. Like the sky, the flight may run past
   * the window's edges; the rocket drifts out of sight and back the way the
   * stars do.
   *
   * It must never cost a reader anything: one rAF loop drives one canvas, the
   * loop stops when the canvas is off screen and does not start at all under
   * `prefers-reduced-motion` (the CSS animation squash cannot reach a canvas,
   * so the gate has to be here), a hidden tab suspends it for free, a parked
   * rocket over a drained trail stops repainting entirely, and between
   * flights there is no loop at all, only the timer.
   */
  const {
    speed = 70,
    trailLife = 7,
    quiet = null,
    active = true,
    edges = undefined,
    slots = undefined,
    sky = null,
  }: {
    /** Top speed, in CSS pixels per second; the flight varies its pace
     * beneath it and never passes it. */
    speed?: number;
    /** Seconds each dash of the trail stays on the sky before dissolving. */
    trailLife?: number;
    /**
     * An element the flight is invisible behind - the page's content panel.
     * The rocket still crosses it, but straight through on the heading it
     * carried in, saving its circles, turns and rests for open sky.
     */
    quiet?: HTMLElement | null;
    /**
     * Whether this home may fly. Flipping it off retires the flight
     * gracefully - departure, then the trail's own fade - never a cut.
     */
    active?: boolean;
    /** The edges the rocket may enter and leave through; edges that lie
     * mid-page are left out by the caller. */
    edges?: CrossingEdge[];
    /** The shared seat budget capping how many easter eggs fly at once. */
    slots?: SkySlots;
    /**
     * The element that stays night on the light page. A retiring flight on
     * a freshly light ground darkens its ink smoothly below this element's
     * fade, so a departure after a dark-to-light switch stays visible all
     * the way out. Without it the ink is starlight always.
     */
    sky?: HTMLElement | null;
  } = $props();

  const TAU = Math.PI * 2;

  /* Starlight inks, sitting in the sky's own palette: the hull and trail on
     the blue-white the stars use, the flame on the amber of its coloured
     ones. The trail is dimmer than the hull so the rocket reads in front of
     its own past. Each carries a dark twin for the light page's ground; a
     retiring flight blends between the two by where it flies. */
  const HULL_INK: Ink = [216, 232, 255];
  const HULL_DARK: Ink = [40, 51, 74];
  const TRAIL_INK: Ink = [186, 212, 255];
  const TRAIL_DARK: Ink = [71, 85, 112];
  const FLAME_CORE: Ink = [255, 214, 140];
  const FLAME_CORE_DARK: Ink = [201, 116, 22];
  const FLAME_EDGE: Ink = [255, 172, 120];
  const FLAME_EDGE_DARK: Ink = [180, 83, 44];

  /* The launch timer: quick after mount so the sky is not long empty, and a
     patient retry when a launch is refused - home inactive, off screen, no
     seat free, reduced motion. */
  const FIRST_MIN_S = 0.3;
  const FIRST_SPAN_S = 0.9;
  const RETRY_MIN_S = 3;
  const RETRY_SPAN_S = 6;

  function strokeDash(
    ctx: CanvasRenderingContext2D,
    dash: TrailDash,
    alpha: number,
    blend: (y: number) => number,
  ): void {
    if (dash.pts.length < 2) return;
    const head = dash.pts[0];
    ctx.strokeStyle = mixInk(TRAIL_INK, TRAIL_DARK, blend(head?.y ?? 0));
    ctx.globalAlpha = alpha * 0.55;
    ctx.beginPath();
    let first = true;
    for (const p of dash.pts) {
      if (first) {
        ctx.moveTo(p.x, p.y);
        first = false;
      } else {
        ctx.lineTo(p.x, p.y);
      }
    }
    ctx.stroke();
  }

  function drawTrail(
    ctx: CanvasRenderingContext2D,
    trail: TrailEmitter,
    time: number,
    life: number,
    blend: (y: number) => number,
  ): void {
    ctx.lineWidth = 1.1;
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    /* Dashes hold full strength for most of their life and dissolve over the
       last stretch - a hard disappearance would tick like a clock at the
       trail's tail end. Each dash takes its ink from where it lies, so a
       trail crossing the sky's fade darkens along its own length. */
    const fadeFrom = life * 0.7;
    for (const dash of trail.dashes()) {
      const age = time - dash.born;
      const alpha = age <= fadeFrom ? 1 : Math.max(0, 1 - (age - fadeFrom) / (life - fadeFrom));
      if (alpha > 0) strokeDash(ctx, dash, alpha, blend);
    }
    const growing = trail.growing();
    if (growing !== null) strokeDash(ctx, growing, 1, blend);
    ctx.globalAlpha = 1;
  }

  function drawFlame(
    ctx: CanvasRenderingContext2D,
    fl: Flight,
    time: number,
    blend: (y: number) => number,
  ): void {
    if (fl.thrust < 0.04) return;
    const ground = blend(fl.y);
    /* A flame the way line art draws one: a teardrop envelope closing to a
       point behind the nozzle, with a shorter, brighter core nested inside.
       Both tips wag and both lengths breathe - the frequencies share no
       factor, so the flicker never settles into a pulse. Length rides the
       thrust: a launch burns long, a cruise burns steady. */
    ctx.save();
    ctx.translate(fl.x, fl.y);
    ctx.rotate(fl.heading);
    ctx.lineWidth = 1.3;
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    ctx.globalAlpha = Math.min(1, fl.thrust * 1.6);
    const flick = 0.82 + 0.18 * Math.sin(time * 21) + 0.07 * Math.sin(time * 47 + 1.7);
    const sway = Math.sin(time * 9.3) * 1.9 + Math.sin(time * 15.7) * 0.7;
    const outer = (11 + 24 * fl.thrust) * flick;
    const bulge = 4.6 + 0.6 * Math.sin(time * 13.1 + 0.8);
    ctx.strokeStyle = mixInk(FLAME_EDGE, FLAME_EDGE_DARK, ground);
    ctx.beginPath();
    ctx.moveTo(-15.5, -3.2);
    ctx.quadraticCurveTo(-15.5 - outer * 0.45, -bulge + sway * 0.3, -15.5 - outer, sway);
    ctx.quadraticCurveTo(-15.5 - outer * 0.45, bulge + sway * 0.3, -15.5, 3.2);
    ctx.stroke();
    const core = outer * 0.55;
    const coreSway = sway * 0.6 + Math.sin(time * 27) * 0.4;
    ctx.strokeStyle = mixInk(FLAME_CORE, FLAME_CORE_DARK, ground);
    ctx.beginPath();
    ctx.moveTo(-15.5, -1.7);
    ctx.quadraticCurveTo(-15.5 - core * 0.5, -2.6 + coreSway * 0.3, -15.5 - core, coreSway);
    ctx.quadraticCurveTo(-15.5 - core * 0.5, 2.6 + coreSway * 0.3, -15.5, 1.7);
    ctx.stroke();
    ctx.restore();
    ctx.globalAlpha = 1;
  }

  function drawRocket(
    ctx: CanvasRenderingContext2D,
    fl: Flight,
    blend: (y: number) => number,
  ): void {
    ctx.save();
    ctx.translate(fl.x, fl.y);
    ctx.rotate(fl.heading);
    ctx.strokeStyle = mixInk(HULL_INK, HULL_DARK, blend(fl.y));
    ctx.globalAlpha = 0.92;
    ctx.lineWidth = 1.4;
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    ctx.beginPath();
    /* Hull: nose at +x, boat tail behind. */
    ctx.moveTo(16, 0);
    ctx.quadraticCurveTo(4, -7.2, -9, -5.2);
    ctx.lineTo(-13, -3.4);
    ctx.lineTo(-13, 3.4);
    ctx.lineTo(-9, 5.2);
    ctx.quadraticCurveTo(4, 7.2, 16, 0);
    /* Fins, swept back past the tail. */
    ctx.moveTo(-4.5, -5.6);
    ctx.lineTo(-14.5, -10);
    ctx.lineTo(-11.5, -4.9);
    ctx.moveTo(-4.5, 5.6);
    ctx.lineTo(-14.5, 10);
    ctx.lineTo(-11.5, 4.9);
    /* Nozzle. */
    ctx.moveTo(-13, -2.6);
    ctx.lineTo(-15.5, -3.2);
    ctx.moveTo(-13, 2.6);
    ctx.lineTo(-15.5, 3.2);
    ctx.stroke();
    /* The porthole. */
    ctx.beginPath();
    ctx.arc(5, 0, 2.1, 0, TAU);
    ctx.stroke();
    ctx.restore();
    ctx.globalAlpha = 1;
  }

  function flight(): Attachment<HTMLCanvasElement> {
    return (canvas) => {
      const ctx = canvas.getContext('2d');
      if (ctx === null) return;

      let fl: Flight | null = null;
      let trail = new TrailEmitter();
      let timer = 0;
      let raf = 0;
      let running = false;
      let seat = false;
      let inView = true;
      let lastTs = 0;
      let simT = 0;
      let cssW = 0;
      let cssH = 0;
      let parkedDrawn = false;
      let frame = 0;

      const reduce = window.matchMedia('(prefers-reduced-motion: reduce)');

      const schedule = (min: number, span: number): void => {
        clearTimeout(timer);
        timer = window.setTimeout(begin, (min + Math.random() * span) * 1000);
      };

      /* An idle canvas holds no bitmap: at these sizes a backing store is
         tens of megabytes, and a home that is merely waiting its turn - the
         inactive theme's, or one between flights - should cost nothing. The
         bitmap exists exactly as long as a flight does. */
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
        if (slots !== undefined && !slots.take()) {
          schedule(RETRY_MIN_S, RETRY_SPAN_S);
          return;
        }
        seat = true;
        trail = new TrailEmitter();
        applyBitmap();
        fl = new Flight({
          bounds: { minX: 0, minY: 0, maxX: cssW, maxY: cssH },
          cruise: speed,
          edges,
          random: Math.random,
        });
        parkedDrawn = false;
        sync();
      };

      const dismiss = (): void => {
        fl = null;
        trail = new TrailEmitter();
        if (seat) {
          seat = false;
          slots?.release();
        }
        releaseBitmap();
        parkedDrawn = false;
      };

      /* The sky's fade, in canvas coordinates: above `fadeStart` the ground
         is night whatever the theme; by `fadeEnd` it is the page. A
         retiring flight blends its ink across this band. */
      let fadeStart = 0;
      let fadeEnd = 0;
      let adaptiveInk = false;

      const blend = (y: number): number => (adaptiveInk ? fadeBlend(y, fadeStart, fadeEnd) : 0);

      /* Where the panel stands and where the sky gives out, in canvas
         coordinates - the quiet zone padded to cover the panel's blurred
         edge. Measured seldom - the layout is still - and never in the same
         frame twice. */
      const syncZones = (): void => {
        if (fl === null) return;
        const own = canvas.getBoundingClientRect();
        if (quiet === null) {
          fl.setQuiet(null);
        } else {
          const zone = quiet.getBoundingClientRect();
          const pad = 12;
          fl.setQuiet({
            minX: zone.left - own.left - pad,
            minY: zone.top - own.top - pad,
            maxX: zone.right - own.left + pad,
            maxY: zone.bottom - own.top + pad,
          });
        }
        if (sky !== null) {
          const patch = sky.getBoundingClientRect();
          fadeStart = patch.top - own.top + patch.height * 0.42;
          fadeEnd = patch.top - own.top + patch.height * 0.8;
        }
      };

      const tick = (ts: number): void => {
        raf = requestAnimationFrame(tick);
        if (fl === null) return;
        frame += 1;
        if (frame % 90 === 1) syncZones();
        /* Starlight while the home is active - its ground is night - and
           position-blended ink for a flight retiring onto the light page. */
        adaptiveInk = !active && sky !== null;
        /* A background tab stops rAF; on return the whole absence arrives as
           one dt, clamped so the rocket resumes where it was rather than
           teleporting through the accumulated time. */
        const dt = Math.min(Math.max((ts - lastTs) / 1000, 0), 0.1);
        lastTs = ts;
        simT += dt;
        /* Props are read here, in the loop rather than in the attachment
           body, so changing one steers the running flight instead of tearing
           the scene down around it. */
        fl.setCruise(speed);
        const life = Math.max(0.5, trailLife);
        if (!active && !fl.departing) fl.depart();
        fl.step(dt);
        if (fl.speed > 1 && !fl.gone) {
          trail.advance(fl.x - Math.cos(fl.heading) * 16, fl.y - Math.sin(fl.heading) * 16, simT);
        } else {
          trail.lift();
        }
        trail.prune(simT, life);
        if (fl.gone && trail.empty) {
          /* Departed and every dash faded: the flight is over off screen.
             The seat goes back, the loop stops, and the timer decides when -
             and whether - the next flight happens. */
          dismiss();
          sync();
          schedule(RETRY_MIN_S, RETRY_SPAN_S);
          return;
        }
        /* A parked rocket over a drained trail is a still picture. */
        const parked = fl.resting && trail.empty;
        if (parked && parkedDrawn) return;
        parkedDrawn = parked;
        ctx.clearRect(0, 0, cssW, cssH);
        drawTrail(ctx, trail, simT, life, blend);
        drawFlame(ctx, fl, simT, blend);
        drawRocket(ctx, fl, blend);
      };

      const sync = (): void => {
        const run = fl !== null && inView && !reduce.matches;
        if (run !== running) {
          running = run;
          if (run) {
            lastTs = performance.now();
            raf = requestAnimationFrame(tick);
          } else {
            cancelAnimationFrame(raf);
          }
        }
        if (reduce.matches && fl !== null) {
          /* Reduced motion means nothing moves, and a rocket frozen
             mid-flight is not nothing moving - the sky goes back to bare.
             The one place the retirement is immediate rather than flown. */
          dismiss();
        }
      };

      const onReduce = (): void => {
        sync();
        clearTimeout(timer);
        if (!reduce.matches) schedule(RETRY_MIN_S, RETRY_SPAN_S);
      };

      const resize = (): void => {
        const w = canvas.clientWidth;
        const h = canvas.clientHeight;
        if (w === 0 || h === 0) return;
        cssW = w;
        cssH = h;
        /* Only a flying home re-sizes its bitmap; an idle one stays bare.
           The cap at 2x: past it the extra pixels cost fill rate and read
           the same through the sky's mask. */
        if (fl === null) return;
        applyBitmap();
        fl.setBounds({ minX: 0, minY: 0, maxX: w, maxY: h });
        parkedDrawn = false;
      };

      const ro = new ResizeObserver(resize);
      ro.observe(canvas);

      /* Scrolled away is the common case on a long page: no reader, no work. */
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
        if (seat) slots?.release();
      };
    };
  }
</script>

<canvas class="night-rocket" aria-hidden="true" {@attach flight()}></canvas>

<style>
  .night-rocket {
    display: block;
    height: 100%;
    pointer-events: none;
    width: 100%;
  }
</style>

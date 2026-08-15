<script lang="ts">
  import type { Attachment } from 'svelte/attachments';

  import { Flight, TrailEmitter, type TrailDash } from '../lib/rocket';

  /**
   * The sky's easter egg: a small line-drawn rocket that wanders wherever
   * this canvas is placed, leaving a dashed trail that remembers the shape of
   * every curve it flew for a while and then dissolves. It cruises, banks,
   * flies the odd full circle or a sudden tight turn, and sometimes glides to
   * a stop, sits, turns on the spot and burns back up to speed. The behaviour
   * lives in `lib/rocket.ts`; this component only sizes the canvas and draws.
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
   * so the gate has to be here), a hidden tab suspends it for free, and a
   * parked rocket over a drained trail stops repainting entirely.
   */
  const {
    speed = 70,
    trailLife = 7,
    quiet = null,
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
  } = $props();

  const TAU = Math.PI * 2;

  /* Starlight inks, sitting in the sky's own palette: the hull and trail on
     the blue-white the stars use, the flame on the amber of its coloured
     ones. The trail is dimmer than the hull so the rocket reads in front of
     its own past. */
  const HULL_INK = 'rgb(216 232 255 / 92%)';
  const TRAIL_INK = 'rgb(186 212 255)';
  const FLAME_CORE = 'rgb(255 214 140)';
  const FLAME_EDGE = 'rgb(255 172 120)';

  function strokeDash(ctx: CanvasRenderingContext2D, dash: TrailDash, alpha: number): void {
    if (dash.pts.length < 2) return;
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
  ): void {
    ctx.strokeStyle = TRAIL_INK;
    ctx.lineWidth = 1.1;
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    /* Dashes hold full strength for most of their life and dissolve over the
       last stretch - a hard disappearance would tick like a clock at the
       trail's tail end. */
    const fadeFrom = life * 0.7;
    for (const dash of trail.dashes()) {
      const age = time - dash.born;
      const alpha = age <= fadeFrom ? 1 : Math.max(0, 1 - (age - fadeFrom) / (life - fadeFrom));
      if (alpha > 0) strokeDash(ctx, dash, alpha);
    }
    const growing = trail.growing();
    if (growing !== null) strokeDash(ctx, growing, 1);
    ctx.globalAlpha = 1;
  }

  function drawFlame(ctx: CanvasRenderingContext2D, fl: Flight, time: number): void {
    if (fl.thrust < 0.04) return;
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
    ctx.strokeStyle = FLAME_EDGE;
    ctx.beginPath();
    ctx.moveTo(-15.5, -3.2);
    ctx.quadraticCurveTo(-15.5 - outer * 0.45, -bulge + sway * 0.3, -15.5 - outer, sway);
    ctx.quadraticCurveTo(-15.5 - outer * 0.45, bulge + sway * 0.3, -15.5, 3.2);
    ctx.stroke();
    const core = outer * 0.55;
    const coreSway = sway * 0.6 + Math.sin(time * 27) * 0.4;
    ctx.strokeStyle = FLAME_CORE;
    ctx.beginPath();
    ctx.moveTo(-15.5, -1.7);
    ctx.quadraticCurveTo(-15.5 - core * 0.5, -2.6 + coreSway * 0.3, -15.5 - core, coreSway);
    ctx.quadraticCurveTo(-15.5 - core * 0.5, 2.6 + coreSway * 0.3, -15.5, 1.7);
    ctx.stroke();
    ctx.restore();
    ctx.globalAlpha = 1;
  }

  function drawRocket(ctx: CanvasRenderingContext2D, fl: Flight): void {
    ctx.save();
    ctx.translate(fl.x, fl.y);
    ctx.rotate(fl.heading);
    ctx.strokeStyle = HULL_INK;
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
  }

  function flight(): Attachment<HTMLCanvasElement> {
    return (canvas) => {
      const ctx = canvas.getContext('2d');
      if (ctx === null) return;

      let fl: Flight | null = null;
      const trail = new TrailEmitter();
      let raf = 0;
      let active = false;
      let inView = true;
      let lastTs = 0;
      let simT = 0;
      let cssW = 0;
      let cssH = 0;
      let parkedDrawn = false;
      let frame = 0;

      const reduce = window.matchMedia('(prefers-reduced-motion: reduce)');

      /* Where the panel stands, in canvas coordinates, padded to cover its
         blurred edge. Measured seldom - the layout is still - and never in
         the same frame twice. */
      const syncQuiet = (): void => {
        if (fl === null) return;
        if (quiet === null) {
          fl.setQuiet(null);
          return;
        }
        const own = canvas.getBoundingClientRect();
        const zone = quiet.getBoundingClientRect();
        const pad = 12;
        fl.setQuiet({
          minX: zone.left - own.left - pad,
          minY: zone.top - own.top - pad,
          maxX: zone.right - own.left + pad,
          maxY: zone.bottom - own.top + pad,
        });
      };

      const tick = (ts: number): void => {
        raf = requestAnimationFrame(tick);
        if (fl === null) return;
        frame += 1;
        if (frame % 90 === 1) syncQuiet();
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
        fl.step(dt);
        if (fl.speed > 1) {
          trail.advance(fl.x - Math.cos(fl.heading) * 16, fl.y - Math.sin(fl.heading) * 16, simT);
        } else {
          trail.lift();
        }
        trail.prune(simT, life);
        /* A parked rocket over a drained trail is a still picture. */
        const parked = fl.resting && trail.empty;
        if (parked && parkedDrawn) return;
        parkedDrawn = parked;
        ctx.clearRect(0, 0, cssW, cssH);
        drawTrail(ctx, trail, simT, life);
        drawFlame(ctx, fl, simT);
        drawRocket(ctx, fl);
      };

      const sync = (): void => {
        const run = inView && !reduce.matches;
        if (run === active) return;
        active = run;
        if (run) {
          lastTs = performance.now();
          raf = requestAnimationFrame(tick);
        } else {
          cancelAnimationFrame(raf);
          if (reduce.matches) {
            /* Reduced motion means nothing moves, and a rocket frozen
               mid-flight is not nothing moving - the sky goes back to bare. */
            ctx.clearRect(0, 0, cssW, cssH);
            parkedDrawn = false;
          }
        }
      };

      const resize = (): void => {
        const w = canvas.clientWidth;
        const h = canvas.clientHeight;
        if (w === 0 || h === 0) return;
        /* Capped: past 2x the extra pixels cost fill rate and read the same
           through the sky's mask. */
        const dpr = Math.min(window.devicePixelRatio || 1, 2);
        canvas.width = Math.round(w * dpr);
        canvas.height = Math.round(h * dpr);
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
        cssW = w;
        cssH = h;
        const bounds = { minX: 0, minY: 0, maxX: w, maxY: h };
        if (fl === null) {
          fl = new Flight({ bounds, cruise: speed, random: Math.random });
        } else {
          fl.setBounds(bounds);
        }
        parkedDrawn = false;
      };

      /* Fires once on observe, which is what creates the flight. */
      const ro = new ResizeObserver(resize);
      ro.observe(canvas);

      /* Scrolled away is the common case on a long page: no reader, no work. */
      const io = new IntersectionObserver((entries) => {
        for (const entry of entries) inView = entry.isIntersecting;
        sync();
      });
      io.observe(canvas);

      reduce.addEventListener('change', sync);
      sync();

      return () => {
        cancelAnimationFrame(raf);
        ro.disconnect();
        io.disconnect();
        reduce.removeEventListener('change', sync);
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

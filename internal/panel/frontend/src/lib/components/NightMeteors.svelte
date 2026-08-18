<script lang="ts">
  import type { Attachment } from 'svelte/attachments';

  import type { CrossingEdge } from '../crossing';
  import { fadeBlend, mixInk, type Ink } from '../ink';
  import { Streak } from '../meteor';
  import type { SkySlots } from '../sky-slots';

  /**
   * The third easter egg: meteors. An appearance is a small shower - one to
   * three streaks, seconds apart, each a straight burn across the sky with
   * a tail streaming behind the head and nothing left where it passed. A
   * streak begins and ends past edges that lie off screen, tail included,
   * so no meteor ever winks out mid-page.
   *
   * A shower takes one seat from the shared `SkySlots` budget - the whole
   * shower, not each streak - and gives it back once its last streak has
   * cleared, so the sky never holds more than two easter eggs at once.
   *
   * The same guarantees as the others: between showers there is no
   * animation frame at all, only one pending timer, which is also what
   * wakes this home again after it has been inactive through a theme
   * switch; the loop stops when the canvas is off screen; nothing runs or
   * shows under `prefers-reduced-motion` (the CSS animation squash cannot
   * reach a canvas, so the gate is here); a hidden tab suspends a shower
   * where it stands.
   */
  const {
    edges,
    active = true,
    slots = undefined,
    firstShower,
    sky = null,
  }: {
    /**
     * The canvas edges a streak may begin and end past. A streak only ever
     * appears and disappears off screen, so a caller whose canvas has an
     * edge lying mid-page leaves that edge out.
     */
    edges?: CrossingEdge[];
    /**
     * Whether this home may start showers. Flipping it off never cuts one
     * short - streaks in flight finish off screen; only new showers wait.
     */
    active?: boolean;
    /** The shared seat budget capping how many easter eggs fly at once. */
    slots?: SkySlots;
    /**
     * Seconds before the first shower may fall, and the span it is drawn from. Left
     * out, the pages get the wait they want. It exists for the catalogue, where a
     * story about the meteors that shows nothing for its first six seconds is not
     * showing what it describes.
     */
    firstShower?: { after: number; within: number };
    /**
     * The element that stays night on the light page. Streaks finishing
     * after a dark-to-light switch darken their ink smoothly below this
     * element's fade, so they stay visible to their off-screen end.
     */
    sky?: HTMLElement | null;
  } = $props();

  /* The head burns white and the tail cools through the stars' blue,
     reaching nothing on its own hue like every glow in this sky - each with
     a dark twin for a streak caught out on the light page. */
  const HEAD_INK: Ink = [255, 255, 255];
  const HEAD_DARK: Ink = [30, 39, 58];
  const TAIL_NEAR: Ink = [235, 245, 255];
  const TAIL_NEAR_DARK: Ink = [40, 51, 74];
  const TAIL_MID: Ink = [214, 230, 255];
  const TAIL_MID_DARK: Ink = [55, 68, 94];
  const TAIL_FAR: Ink = [190, 214, 255];
  const TAIL_FAR_DARK: Ink = [71, 85, 112];

  const FIRST_MIN_S = 6;
  const FIRST_SPAN_S = 18;
  const NEXT_MIN_S = 18;
  const NEXT_SPAN_S = 40;
  const RETRY_MIN_S = 5;
  const RETRY_SPAN_S = 10;

  function drawStreak(ctx: CanvasRenderingContext2D, streak: Streak, ground: number): void {
    const tailX = streak.x - streak.ux * streak.tail;
    const tailY = streak.y - streak.uy * streak.tail;
    /* One gradient per streak per frame - two or three at the worst - so
       the tail dims along its own length rather than in steps. */
    const glow = ctx.createLinearGradient(streak.x, streak.y, tailX, tailY);
    glow.addColorStop(0, mixInk(TAIL_NEAR, TAIL_NEAR_DARK, ground, 90));
    glow.addColorStop(0.3, mixInk(TAIL_MID, TAIL_MID_DARK, ground, 45));
    glow.addColorStop(1, mixInk(TAIL_FAR, TAIL_FAR_DARK, ground, 0));
    ctx.strokeStyle = glow;
    ctx.lineWidth = 1.4;
    ctx.beginPath();
    ctx.moveTo(streak.x, streak.y);
    ctx.lineTo(tailX, tailY);
    ctx.stroke();
    /* The head: a short hot dash riding the front of the burn. */
    ctx.strokeStyle = mixInk(HEAD_INK, HEAD_DARK, ground, 95);
    ctx.lineWidth = 1.7;
    ctx.beginPath();
    ctx.moveTo(streak.x, streak.y);
    ctx.lineTo(streak.x - streak.ux * 4.5, streak.y - streak.uy * 4.5);
    ctx.stroke();
  }

  function shower(): Attachment<HTMLCanvasElement> {
    return (canvas) => {
      const ctx = canvas.getContext('2d');
      if (ctx === null) return;

      let streaks: Streak[] = [];
      let pending = 0;
      let nextLaunch = 0;
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
         Measured when a shower starts and on resize. */
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
         of megabytes, and showers are rare. The bitmap exists exactly as
         long as a shower does. */
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
        /* The sky seats two; a full house costs this shower one retry. */
        if (slots !== undefined && !slots.take()) {
          schedule(RETRY_MIN_S, RETRY_SPAN_S);
          return;
        }
        seat = true;
        applyBitmap();
        syncFade();
        streaks = [new Streak({ width: cssW, height: cssH, edges, random: Math.random })];
        pending = Math.floor(Math.random() * 3);
        nextLaunch = simT + 0.4 + Math.random() * 1.4;
        sync();
      };

      const releaseSeat = (): void => {
        if (!seat) return;
        seat = false;
        slots?.release();
      };

      const tick = (ts: number): void => {
        raf = requestAnimationFrame(tick);
        if (streaks.length === 0 && pending === 0) return;
        /* Clamped like the others: a hidden tab hands the whole absence
           back as one dt, and the shower resumes instead of skipping. */
        const dt = Math.min(Math.max((ts - lastTs) / 1000, 0), 0.1);
        lastTs = ts;
        simT += dt;
        if (pending > 0 && simT >= nextLaunch) {
          streaks.push(new Streak({ width: cssW, height: cssH, edges, random: Math.random }));
          pending -= 1;
          nextLaunch = simT + 0.4 + Math.random() * 1.4;
        }
        ctx.clearRect(0, 0, cssW, cssH);
        /* Starlight while the home is active; streaks finishing on the
           light page blend dark as they move below the sky's fade. */
        const adaptive = !active && sky !== null;
        let alive = 0;
        for (const streak of streaks) {
          streak.step(dt);
          if (!streak.done) {
            drawStreak(ctx, streak, adaptive ? fadeBlend(streak.y, fadeStart, fadeEnd) : 0);
            alive += 1;
          }
        }
        if (alive < streaks.length) streaks = streaks.filter((streak) => !streak.done);
        if (streaks.length === 0 && pending === 0) {
          /* The last streak has cleared, off screen: the shower is over. */
          releaseSeat();
          releaseBitmap();
          sync();
          schedule(NEXT_MIN_S, NEXT_SPAN_S);
        }
      };

      const sync = (): void => {
        const run = (streaks.length > 0 || pending > 0) && inView && !reduce.matches;
        if (run !== running) {
          running = run;
          if (run) {
            lastTs = performance.now();
            raf = requestAnimationFrame(tick);
          } else {
            cancelAnimationFrame(raf);
          }
        }
        if (reduce.matches && (streaks.length > 0 || pending > 0)) {
          /* Reduced motion mid-shower: the sky simply has no meteors. */
          streaks = [];
          pending = 0;
          releaseSeat();
          releaseBitmap();
        }
      };

      const onReduce = (): void => {
        sync();
        clearTimeout(timer);
        if (!reduce.matches) schedule(NEXT_MIN_S, NEXT_SPAN_S);
      };

      const resize = (): void => {
        const w = canvas.clientWidth;
        const h = canvas.clientHeight;
        if (w === 0 || h === 0) return;
        cssW = w;
        cssH = h;
        /* Only a shower in flight re-sizes its bitmap; idle stays bare. */
        if (streaks.length > 0 || pending > 0) {
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
      schedule(firstShower?.after ?? FIRST_MIN_S, firstShower?.within ?? FIRST_SPAN_S);

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

<canvas class="night-meteors" aria-hidden="true" {@attach shower()}></canvas>

<style>
  .night-meteors {
    display: block;
    height: 100%;
    pointer-events: none;
    width: 100%;
  }
</style>

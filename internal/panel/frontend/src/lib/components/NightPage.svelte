<script lang="ts">
  import type { Snippet } from 'svelte';

  import type { PanelBuild } from '../base';
  import { SkySlots } from '../sky-slots';
  import {
    applyDocumentTheme,
    DEFAULT_THEME_DISPLAY,
    isThemeDisplay,
    resolveThemeDisplay,
    systemThemeDisplay,
    type ThemeDisplay,
  } from '../preferences';
  import { createPrefsSync } from '../preferences-sync';
  import BrandMark from './BrandMark.svelte';
  import NightAstronaut from './NightAstronaut.svelte';
  import NightMeteors from './NightMeteors.svelte';
  import NightRocket from './NightRocket.svelte';
  import NightSky from './NightSky.svelte';
  import PageFooter from './PageFooter.svelte';
  import ThemeSwitch from './ThemeSwitch.svelte';

  const {
    title,
    documentTitle,
    build,
    busy = false,
    size = 'default',
    children,
  }: {
    /** Names whichever state the card is showing. Stands above it, as its head. */
    title: string;
    /** Leading segment of the document title; the panel's name is appended here. */
    documentTitle: string;
    build: PanelBuild;
    /** Marks the card busy and shows the progress cursor over it. */
    busy?: boolean;
    /**
     * How much room the card is given. `compact` is for a card that holds a
     * sentence and a button rather than a set of facts: the column narrows and the
     * floor drops, so a short card reads as deliberately small instead of as a
     * full-width one that came up short.
     */
    size?: 'default' | 'compact';
    children: Snippet;
  } = $props();

  /* The mark's size. Large enough to be the page's subject rather than a badge on
     it, which is what the sky is drawn around. */
  const MARK_SIZE = 104;

  /* The sky measures itself against the gap above the card, and a compact card
     leaves a much larger one, so the same percentage reaches further down and
     the fade ends up under the footer rather than above it: at 1280x900 the
     invitation's sky finishes 79px above the footer and the compact page's
     finished 120px below it, taking the footer's host line to 4.12:1.
     It is squeezed from both ends. The title is light ink and needs dark sky
     behind it, the footer is dark ink and needs to be clear of the fade, and a
     short card puts the two closer together than any other page does. Swept
     200-480% against both, in light, at 390x844, 768x700, 1280x900 and
     1920x1200: below 350% the title falls to 3.32:1 on a phone, the 480% default
     drops the footer to 4.12:1, and 400% is the one value where everything
     clears - worst case 4.63:1.
     `undefined` leaves NightSky's own default, so the pages that want it keep
     the number in one place. */
  const skyHeight = $derived(size === 'compact' ? 'clamp(44rem, 400%, 72rem)' : undefined);

  /* The same synced document the panel writes, without the stream behind it: a
     write here stays pending in local storage and goes up on the first connect
     after signing in, so the theme chosen out here is the one that greets the
     reader inside. */
  const prefs = createPrefsSync();

  /* Read once and never watched. The page opens on whatever the system asks for,
     but it opens on it as a choice already made - the switch shows light or dark
     picked from the first paint, and the page holds it. A `MediaQuery` would
     repaint the page under a reader midway through an invitation because their
     laptop reached sunset, and these are the pages with no account behind them to
     remember what they would rather have. */
  const systemAtOpen = systemThemeDisplay();

  let theme = $state<ThemeDisplay>(storedTheme());
  const resolvedTheme = $derived(resolveThemeDisplay(theme, systemAtOpen));

  /* The card, handed to the dark-mode rocket as the region it is invisible
     behind - it crosses there in a straight line instead of performing to
     nobody. */
  let cardElement = $state<HTMLElement | null>(null);

  /* One seat budget for the whole page: the sky band and the dark overlay
     share it, so at most two easter eggs are ever on screen - including
     mid-switch, when the old home's flight is still leaving while the new
     home's waits for its seat. */
  const flightSlots = new SkySlots();

  /* The sky's element, handed to the overlay flights as the one region that
     stays night after a switch to the light theme: a flight retiring across
     the fresh light page darkens its ink below the sky's fade, so it is
     visible all the way out. */
  let skyElement = $state<HTMLElement | null>(null);

  const darkFlight = $derived(resolvedTheme === 'dark');

  $effect(() => {
    applyDocumentTheme(document, resolvedTheme);
  });

  function storedTheme(): ThemeDisplay {
    const value = prefs.get('theme');
    return typeof value === 'string' && isThemeDisplay(value) ? value : DEFAULT_THEME_DISPLAY;
  }

  function selectTheme(nextTheme: ThemeDisplay): void {
    theme = nextTheme;
    prefs.set('theme', nextTheme);
  }
</script>

<!--
@component
The panel's page for a reader who is not inside the panel: the mark standing
in a night sky, a title, and one card under it.

Two things reach people who have no session - an invitation, and whatever the
server answered when something went wrong - and both are the first thing some
readers ever see of Smyklot. They share this shell so they cannot drift into
two different products, and so the sky, the card's glass and the theme switch
are written once.
-->

<svelte:head>
  <title>{documentTitle} | SMYKLOT</title>
</svelte:head>

<main class={['shell', 'night-shell', size === 'compact' && 'night-compact']}>
  <div class="night-brand">
    <NightSky
      height={skyHeight}
      rocket={!darkFlight}
      astronaut={!darkFlight}
      meteors={!darkFlight}
      slots={flightSlots}
      bind:skyElement
    />
    <!-- Open, so the sky carries through the ring: out here the mark stands on
         night in both themes, which is exactly the ground the robot wants. -->
    <BrandMark stacked interior="clear" size={MARK_SIZE} />
  </div>

  <!-- Both homes exist in both themes; the theme decides which is active.
     Nothing unmounts on a switch, so a flight in progress retires on its own
     terms - the rocket departs, crossings finish off screen - while the new
     home's flights arrive. After the sky in source order: both sit at
     z-index -1, where document order decides, and the flights belong above
     the stars they fly through - still behind every piece of the content. -->
  <div class="page-flight" aria-hidden="true">
    <NightRocket quiet={cardElement} active={darkFlight} slots={flightSlots} sky={skyElement} />
    <NightAstronaut active={darkFlight} slots={flightSlots} sky={skyElement} />
    <NightMeteors active={darkFlight} slots={flightSlots} sky={skyElement} />
  </div>

  <div class="night-main">
    <div class="night-head">
      <h1 class="night-title" id="night-page-title">{title}</h1>
      <ThemeSwitch
        name="night-page-theme"
        theme={resolvedTheme}
        surface="night"
        system={false}
        onSelect={selectTheme}
      />
    </div>

    <section
      bind:this={cardElement}
      class={['plate', 'night-card', busy && 'busy']}
      aria-labelledby="night-page-title"
      aria-busy={busy}
    >
      <div class="plate-body">
        {@render children()}
      </div>
    </section>

    <PageFooter {build} />
  </div>
</main>

<style>
  /* Three rows, and the mark shares the top one with the empty bottom one. Both
     flexible rows take the same share, so the group between them keeps the exact
     centre it had before the mark moved above it - the mark grows into the space
     that was already there rather than pushing the card down. When the content
     outgrows the viewport the flexible rows collapse and the page scrolls from
     the top, so nothing lands above the scroll origin. */
  .night-shell {
    /* Smaller than the panel's own compact control. There is one of these on the
       whole page and it is not what the reader came for, so it steps back from
       the title it shares a row with rather than matching it. */
    --night-switch-height: 1.75rem;
    /* The two numbers `size` sets, kept together so the pair stays a pair: a
       narrow column with a tall floor is a card with a hole in it, and a wide one
       with a short floor is a letterbox. */
    --night-column: 42rem;
    --night-card-floor: 19rem;

    display: grid;
    grid-template-rows: 1fr auto 1fr;
    max-width: var(--night-column);
    min-height: 100dvh;
    padding-block: var(--space-6);
    row-gap: var(--space-6);
  }

  /* A card holding one sentence and one button. Narrow enough that the button is
     never marooned at the end of a line it could have shared, and short enough
     that the card is not mostly air - which is what the default floor, sized for
     an invitation's four rows of facts, would leave it. */
  .night-compact {
    --night-column: 30rem;
    --night-card-floor: 0;
  }

  /* In the dark theme the whole page is night, so the easter eggs fly the full
     window, still behind all of the content. */
  .page-flight {
    inset: 0;
    pointer-events: none;
    position: fixed;
    z-index: -1;
  }

  .page-flight :global(canvas) {
    inset: 0;
    position: absolute;
  }

  /* Stretched to fill its row rather than centred inside it, so the element's own
     height *is* the whitespace above the page's content. That is what the sky
     measures itself against, and the mark sits in the middle of it.

     Nothing is discounted from that middle. It used to carry the head row's
     height as padding, which centred the mark on the gap up to the *card* and so
     left it half that padding low against the whitespace a reader actually sees -
     the row of title and switch reads as the card's own head, not as part of the
     space above it. The mark is one object, icon and wordmark together, and it is
     the object that gets centred: with the padding there the icon looked about
     right and SMYKLOT hung below the middle, which is what gave it away. */
  .night-brand {
    align-items: center;
    align-self: stretch;
    display: flex;
    justify-content: center;
    position: relative;
  }

  /* Centred on the mark, and its height is read from this row - which is the gap
     above the card. The page is centred, so that gap grows when the card is short
     and shrinks when it is tall; a sky measured in rem or `vh` lands differently
     in each state, and whichever line it leaves inside its fade reads against a
     mid-tone. As a multiple of the gap, the title sits at the same point on the
     falloff every time. */
  .night-brand :global(.night-sky) {
    left: 50%;
    top: 50%;
    translate: -50% -50%;
  }

  /* The sky is a viewport wide and centred on a column that is narrower, so it
     reaches the window's edges - and past them once a scrollbar takes a slice out
     of the content box. Clipped rather than hidden, which would make a scroll
     container of the page, and scoped to this page by what it contains so the
     panel's own horizontal scrollers are left alone. */
  :global(html:has(.night-shell)) {
    overflow-x: clip;
  }

  /* Everything outside the card stands on the sky, and the sky is night whichever
     theme the page is in, so this page writes in light ink in both. The card
     keeps the page's own palette: it is a panel laid on the sky, not part of it. */
  .night-brand :global(.mark-name) {
    color: rgb(246 249 255);
  }

  /* The card's own head, lifted out of it: the title on the left names whichever
     state the card is showing, and the switch on the right is the one control on
     the page that is not part of that state. The row keeps the control's height
     whatever the title does, so the gap the mark measures itself against does not
     move when the title wraps. */
  .night-head {
    align-items: center;
    display: flex;
    gap: var(--space-3);
    justify-content: space-between;
    margin-bottom: var(--space-3);
    min-height: var(--night-switch-height);
  }

  /* The control reads its own height from this, so the row and the control cannot
     disagree about how tall the head is. */
  .night-head :global(fieldset) {
    --local-control-height: var(--night-switch-height);
  }

  /* Reads as the card's own title from the outside, so it keeps the size the
     plate header gave it. */
  .night-title {
    color: rgb(246 249 255);
    font: 700 1.0625rem / var(--leading-body) var(--sans);
    letter-spacing: 0;
    margin: 0;
    min-width: 0;
  }

  /* SegmentedControl deliberately fills a phone-width row. Give it that row
     instead of asking the title to shrink beneath it. */
  @media (max-width: 36rem) {
    .night-head {
      align-items: stretch;
      flex-direction: column;
    }
  }

  /* A floor under the card, so its states are not several different page layouts.
     It stops the stack resettling when a load finishes, and it keeps the gap above
     the card - which is what the sky measures itself against - within a narrow
     range instead of doubling when the card holds one line.

     The one thing on the page that is not the sky, so it is not quite opaque
     either: the sky reads through it and the card sits *in* the scene rather than
     on top of it. The blur behind is what makes that safe - it takes the stars out
     of the ground the text stands on, leaving an even wash instead of specks of
     white under the type.

     The lift is what pays for the rest. Straight translucency costs contrast,
     because the sky is denser at the top of the card than at the bottom and the
     type at the top ends up standing on the darkest ground: at 92% opaque and no
     lift, `dt` - the dimmest type on the card - fell to 4.81:1 against a 4.5
     floor, and that was already as far as it could go. Brightening the backdrop
     before it shows through separates the two: the card transmits the sky's
     *shape* without transmitting its darkness, so it can be a great deal more
     see-through and read better while doing it. Measured on the light page across
     620-1600px window heights - see the commit for the numbers.

     The dark page lifts the other way. Its surface is already close to the sky, so
     brightening the backdrop would erase the difference the effect is made of;
     dropping it instead makes the sky behind the card read as depth. */
  .night-card {
    --night-card-lift: 1.6;

    backdrop-filter: blur(22px) saturate(1.4) brightness(var(--night-card-lift));
    background: color-mix(in srgb, var(--surface-base) 86%, transparent);
    border-color: var(--dialog-border);
    box-shadow: var(--shadow-plate);
    margin-bottom: 0;
    min-height: var(--night-card-floor);
  }

  :global(:root[data-theme='dark']) .night-card {
    --night-card-lift: 0.72;
  }

  .night-card :global(.plate-body) {
    align-content: center;
    display: grid;
    min-height: inherit;
    padding: var(--space-5);
  }

  .night-card.busy {
    cursor: progress;
  }

  /* No rule above it: the card's own edge already separates the footer from the
     page's content, and a second line so close to it only crowds the corner. */
  .night-shell :global(.foot) {
    border-top: 0;
    margin-top: var(--space-4);
    padding-top: 0;
  }
</style>

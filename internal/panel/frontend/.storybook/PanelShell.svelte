<script lang="ts">
  import { QueryClientProvider } from '@tanstack/svelte-query';
  import type { Snippet } from 'svelte';

  import { createPanelQueryClient } from '#lib/query-client.js';
  import { PanelSession, setPanelSession } from '#lib/session.svelte.js';
  import { TARGET } from '../stories/support/fixtures.js';
  import { fixtureApi } from '../stories/support/api.js';
  import { applyDocumentTheme, resolveThemeDisplay } from '#lib/preferences.js';
  import type { ThemeDisplay } from '#lib/preferences.js';

  const {
    theme,
    console: consoleMode,
    bleed = false,
    children,
  }: {
    theme: ThemeDisplay;
    /** Which of the two consoles a story is standing in; they are different palettes. */
    console: 'panel' | 'root';
    /**
     * Render into the shell itself rather than the centred content column, for the
     * page backdrops that are sized `100vw`. Set it with `parameters: { bleed: true }`.
     */
    bleed?: boolean;
    children: Snippet;
  } = $props();

  const queryClient = createPanelQueryClient();

  /*
   * A session, because two components read one.
   * ------------------------------------------
   * `RepositoryList` and `InstallationView` call `getPanelSession()`, and a component
   * that asks for a context nobody set does not degrade - Svelte throws
   * `missing_context` and the story renders as a blank frame with the reason only in
   * the console. It is set here rather than per story so a component that starts
   * reading it later does not silently take a catalogue page down with it.
   *
   * Its API answers from the mock's fixtures rather than refusing. A component that
   * takes `api` as a prop gets whatever its story hands it, and refusing everything
   * else is right there; but a component that reads `session.api` gives its story no
   * say - `InstallationView` reaches twenty methods - so refusing drew a shell over
   * nothing. Reads answer from the same data the dev server serves; writes still
   * refuse, because a story is a picture of a state and a mutation that "succeeded"
   * against a fixture would show a result no service produced.
   */
  const session = new PanelSession(fixtureApi(), { version: null, serviceHost: null }, queryClient);

  /*
   * A workspace, because a session with none cannot build an address.
   * ----------------------------------------------------------------
   * `session.repositoryHref()` resolves `/i/[account]/...` from
   * `selectedTarget?.account.login ?? ''`, and SvelteKit refuses an empty parameter -
   * so with no target selected every repository row throws "Missing parameter
   * 'account'" BEFORE it renders, and the table draws its header over nothing. The
   * error names a route, which sends you looking for a router; the cause is that
   * nothing is selected.
   *
   * Worth knowing what does NOT fix it: `parameters.sveltekit_experimental.state.page`
   * supplies `$app/state`, and this address never goes through it.
   */
  session.targets = [TARGET];
  session.selectedId = TARGET.id;
  setPanelSession(session);

  // The app's own function, so the toolbar and the panel can never disagree about
  // what a theme means. It writes `data-theme` on the document element and rewrites
  // the `theme-color` metas, which is the whole of what the panel does at runtime.
  $effect(() => {
    applyDocumentTheme(document, resolveThemeDisplay(theme), consoleMode === 'root');
  });
</script>

<!--
  A `<div>`, where the app's own layout makes this a `<main>`. `.app-shell` is a
  class and carries the palette either way, but the landmark is not the shell's to
  claim here: fifteen stories are themselves pages with a `<main>` of their own, and
  two mains in one document is three axe violations apiece - `landmark-unique`,
  `landmark-no-duplicate-main` and `landmark-main-is-top-level`, 45 of the 54 the
  catalogue had. The app has one `<main>` because it has one page in it.

  `.app-shell` is load-bearing twice over, which is why every story is inside one.

  It carries the Root console palette: `app.css` re-declares the entire alias set on
  `.app-shell.root-mode`, because custom properties resolve at computed-value time and
  cannot be inherited into it. And it is the portal target - `Modal.svelte` is
  `<Dialog.Portal to=".app-shell">`, so without this element every overlay in the
  catalogue would render unthemed or not at all.
-->
<QueryClientProvider client={queryClient}>
  <div class="app-shell" class:root-mode={consoleMode === 'root'}>
    {#if bleed}
      {@render children()}
    {:else}
      <div class="workspace">
        <div class="workspace-content">
          {@render children()}
        </div>
      </div>
    {/if}
  </div>
</QueryClientProvider>

<style>
  /*
    Two overrides, and only two, both of chrome rather than of anything a component
    draws.

    `.app-shell` is a two-column grid - sidebar, then workspace - so a story dropped
    straight into it renders in the sidebar's column and comes out `--sidebar-width`
    wide. And both the shell and the workspace stand `100dvh` tall, which under a
    catalogue means a chip sits at the top of an empty screen.

    Everything that decides how a component *looks* is left alone: the story sits in
    `.workspace-content`, so it gets the same padding, the same `--content-max` and
    the same centring as a real page, and a width measured here is a width the app
    would give it.

    Unless it is a backdrop, which `bleed` is for. `.workspace-content` is a
    centred `--content-max` column, so at a wide window it starts a couple of
    hundred pixels in - and a component sized `100vw`, which is how the app's page
    backdrops are drawn, then begins at that inset and hangs off the right by
    exactly as much. Those components are not page content and the app never puts
    them in this column: `NightPage` is the whole window. `bleed` renders them
    straight into the shell, where `100vw` means what it says.

    These rules carry this component's scope class, which puts them above `app.css`
    by one class. That is the usual trap and here it is the point - but it is also
    why this file may never grow a rule about anything a component paints.
  */
  .app-shell {
    grid-template-columns: minmax(0, 1fr);
    min-height: auto;
  }
  .workspace {
    min-height: auto;
    padding-bottom: var(--space-6);
  }
</style>

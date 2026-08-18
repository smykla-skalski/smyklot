<script lang="ts">
  import { QueryClientProvider } from '@tanstack/svelte-query';
  import type { Snippet } from 'svelte';

  import { createPanelQueryClient } from '#lib/query-client.js';
  import { applyDocumentTheme, resolveThemeDisplay } from '#lib/preferences.js';
  import type { ThemeDisplay } from '#lib/preferences.js';

  const {
    theme,
    console: consoleMode,
    children,
  }: {
    theme: ThemeDisplay;
    /** Which of the two consoles a story is standing in; they are different palettes. */
    console: 'panel' | 'root';
    children: Snippet;
  } = $props();

  const queryClient = createPanelQueryClient();

  // The app's own function, so the toolbar and the panel can never disagree about
  // what a theme means. It writes `data-theme` on the document element and rewrites
  // the `theme-color` metas, which is the whole of what the panel does at runtime.
  $effect(() => {
    applyDocumentTheme(document, resolveThemeDisplay(theme), consoleMode === 'root');
  });
</script>

<!--
  `.app-shell` is load-bearing twice over, which is why every story is inside one.

  It carries the Root console palette: `app.css` re-declares the entire alias set on
  `.app-shell.root-mode`, because custom properties resolve at computed-value time and
  cannot be inherited into it. And it is the portal target - `Modal.svelte` is
  `<Dialog.Portal to=".app-shell">`, so without this element every overlay in the
  catalogue would render unthemed or not at all.
-->
<QueryClientProvider client={queryClient}>
  <main class="app-shell" class:root-mode={consoleMode === 'root'}>
    <div class="workspace">
      <div class="workspace-content">
        {@render children()}
      </div>
    </div>
  </main>
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

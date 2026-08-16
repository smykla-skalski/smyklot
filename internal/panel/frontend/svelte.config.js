import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

export default {
  preprocess: vitePreprocess(),
  kit: {
    adapter: adapter({
      pages: 'dist',
      assets: 'dist',
      fallback: 'index.html',
    }),
    paths: {
      // Sent over the wire as a literal, resolved by the Go server at startup.
      base: '/__smyklot_panel_base__',
    },
  },
};

<script lang="ts">
  import { initializePanel } from '../lib/boot';
  import '../app.css';

  initializePanel(document);

  const { children } = $props();
</script>

{@render children()}

<script lang="ts">
  import Plate from './Plate.svelte';

  const { href }: { href: string } = $props();
</script>

<Plate label="Sign in" tone="lead">
  <p>
    Sign in with GitHub to manage organizations you own and repositories from personal
    installations. Smyklot verifies ownership again before every change
  </p>
  <a class="btn btn-signal" {href} rel="nofollow">Sign in with GitHub</a>
</Plate>

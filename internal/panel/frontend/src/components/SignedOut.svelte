<script lang="ts">
  import Plate from './Plate.svelte';

  const { href }: { href: string } = $props();
</script>

<Plate label="Sign in" tone="lead">
  <p>
    Sign in with GitHub to get a pairing link for a new device. An administrator decides who can
    generate one
  </p>
  <a class="btn btn-signal" {href} rel="nofollow">Sign in with GitHub</a>
</Plate>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLAnchorAttributes } from 'svelte/elements';

  const { href, children, ...rest }: { href: string; children: Snippet } & HTMLAnchorAttributes =
    $props();
</script>

<!--
@component
A link inside a sentence. The panel's other anchors are controls wearing `.btn`, or
rows, or navigation; this is the one that reads as a link in running text, and
`app.css` had carried its underline treatment all along with no component to ask for
it.

Rest props pass through because every use of this is external: the invitation page
links two GitHub profiles, and both need a `target` and `rel` of their own.
-->

<a class="link" {href} {...rest}>{@render children()}</a>

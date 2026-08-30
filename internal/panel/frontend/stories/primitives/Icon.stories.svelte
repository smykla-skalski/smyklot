<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Icon, { type IconName, type IconSize } from '#lib/components/Icon.svelte';

  /** The scale, smallest first, so the control walks it in order. */
  const SIZES: IconSize[] = ['nano', 'micro', 'xs', 'sm', 'base', 'md'];

  /**
   * Every name the union declares, in the order it declares them.
   *
   * Written out rather than derived, because that is what makes a gap visible from
   * both directions: TypeScript refuses the list if a name leaves the union, and the
   * gallery shows an empty cell if a glyph leaves the switch.
   */
  const NAMES: IconName[] = [
    'admin',
    'alert',
    'ban',
    'branch',
    'chevron-down',
    'chevron-left',
    'chevron-right',
    'chevron-up',
    'chevrons-up-down',
    'circle-dashed',
    'circle-slash',
    'close',
    'editor',
    'failure',
    'file',
    'file-bypassed',
    'file-invalid',
    'file-missing',
    'file-valid',
    'filter',
    'github',
    'globe',
    'history',
    'info',
    'link',
    'link-off',
    'lock',
    'mail',
    'more',
    'moon',
    'minus-circle',
    'no-access',
    'notifications',
    'organization',
    'owner',
    'pending',
    'plus',
    'repositories',
    'refresh',
    'search',
    'settings',
    'shield',
    'shield-slash',
    'sidebar-collapse',
    'sidebar-expand',
    'sign-out',
    'sliders',
    'sort',
    'check',
    'success',
    'sun',
    'sun-moon',
    'system',
    'trash',
    'user',
    'user-plus',
    'users',
    'viewer',
    'warning',
  ];

  const { Story } = defineMeta({
    title: 'Primitives/Icon',
    component: Icon,
    argTypes: {
      name: { control: 'select', options: NAMES },
      size: { control: 'inline-radio', options: SIZES },
      strokeWidth: { control: { type: 'range', min: 1, max: 3, step: 0.25 } },
    },
    args: { name: 'admin', size: 'base', strokeWidth: 1.75 },
  });
</script>

<Story name="Playground">
  {#snippet template(args)}
    <Icon {...args} />
  {/snippet}
</Story>

<Story name="Gallery">
  {#snippet template(args)}
    <ul class="gallery">
      {#each NAMES as name (name)}
        <li>
          <Icon {name} size={args.size} strokeWidth={args.strokeWidth} />
          <code class="mono">{name}</code>
        </li>
      {/each}
    </ul>
  {/snippet}
</Story>

<style>
  .gallery {
    display: grid;
    gap: var(--space-3);
    grid-template-columns: repeat(auto-fill, minmax(11rem, 1fr));
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .gallery li {
    align-items: center;
    color: var(--text-muted);
    display: flex;
    gap: var(--space-2);
  }
  .gallery code {
    font-size: var(--font-size-meta);
  }
</style>

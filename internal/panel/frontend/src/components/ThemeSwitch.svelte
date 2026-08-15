<script lang="ts">
  import { isThemeDisplay, type ThemeDisplay } from '../lib/preferences';
  import SegmentedControl from './SegmentedControl.svelte';

  /* Icon-only, so each option's label is its accessible name rather than visible text. */
  const THEME_OPTIONS = [
    { value: 'system', label: 'System theme', icon: 'system' },
    { value: 'light', label: 'Light theme', icon: 'sun' },
    { value: 'dark', label: 'Dark theme', icon: 'moon' },
  ] as const;

  const {
    name,
    theme,
    onSelect,
    surface = 'panel',
  }: {
    /** The radio group's name, which has to be unique on the page. */
    name: string;
    theme: ThemeDisplay;
    onSelect: (theme: ThemeDisplay) => void;
    /** Which family of surfaces to draw on, passed straight to the control. */
    surface?: 'panel' | 'sidebar' | 'night';
  } = $props();
</script>

<SegmentedControl
  {name}
  label="Theme"
  options={THEME_OPTIONS}
  value={theme}
  {surface}
  compact
  onSelect={(selection) => {
    if (isThemeDisplay(selection)) onSelect(selection);
  }}
/>

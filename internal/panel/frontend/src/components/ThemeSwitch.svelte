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
    system = true,
  }: {
    /** The radio group's name, which has to be unique on the page. */
    name: string;
    theme: ThemeDisplay;
    onSelect: (theme: ThemeDisplay) => void;
    /** Which family of surfaces to draw on, passed straight to the control. */
    surface?: 'panel' | 'sidebar' | 'night';
    /**
     * Whether to offer "follow the system". Somewhere that cannot keep the answer -
     * a page reached before signing in - is better off asking for a theme outright
     * than offering to follow something it will forget.
     */
    system?: boolean;
  } = $props();

  const options = $derived(
    system ? THEME_OPTIONS : THEME_OPTIONS.filter((option) => option.value !== 'system'),
  );
</script>

<SegmentedControl
  {name}
  label="Theme"
  {options}
  value={theme}
  {surface}
  compact
  onSelect={(selection) => {
    if (isThemeDisplay(selection)) onSelect(selection);
  }}
/>

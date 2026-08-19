/**
 * Whether a click on a link is one the panel should answer itself.
 *
 * Every SPA link in the panel is a real `<a href>` first: the address is right,
 * middle-click opens a tab, copy-link copies something that works. The handler
 * on top of it takes over only the plain case, and has to let every other one
 * through untouched - a Cmd-click that calls `preventDefault()` is a new tab
 * that never opens, and it fails silently, which is why it survives.
 *
 * Written out at nine call sites before this, and they had already drifted:
 * some checked `shiftKey` and `altKey`, some did not; some checked
 * `defaultPrevented`, most did not. Two components added later - `SectionTabs`
 * and `Crumb` - carried the comment about keeping the href real and none of the
 * guard, so four strips across the panel could not be opened in a new tab at
 * all.
 *
 * `defaultPrevented` is in because a handler nearer the click may already have
 * answered it; the four modifiers are in because each one means "not here";
 * and `button !== 0` covers middle and right, which browsers report through
 * this same event.
 */
export function plainClick(event: MouseEvent): boolean {
  return (
    !event.defaultPrevented &&
    event.button === 0 &&
    !event.metaKey &&
    !event.ctrlKey &&
    !event.shiftKey &&
    !event.altKey
  );
}

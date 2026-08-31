/**
 * A workspace's generated mark: the hue it is drawn in, and the letters on it.
 *
 * Its own module rather than the rail's, because the rail stopped being the only
 * place a workspace is drawn - the collapsed sidebar carries the same mark, and the
 * menu behind it does too. Two copies of a hash is two workspaces that are the same
 * colour in one place and not in another.
 */

/**
 * The identity hue: hashed once from the login and rendered as `data-h`.
 * The stylesheet does everything else - tint, line, ink and the selected
 * aurora all derive from this one number in OKLCH.
 */
export function workspaceHue(login: string): number {
  let hash = 5381;
  for (let i = 0; i < login.length; i += 1) {
    hash = (hash * 33) ^ login.charCodeAt(i);
  }
  return (hash >>> 0) % 360;
}

/** "Smykla Skalski" -> "SS", "bartsmykla" -> "B", "Oak & Pine" -> "OP". */
export function workspaceInitials(name: string): string {
  const words = name.split(/[^\p{L}\p{N}]+/u).filter((word) => word.length > 0);
  if (words.length === 0) return '?';
  return words
    .slice(0, 2)
    .map((word) => word[0]!.toUpperCase())
    .join('');
}

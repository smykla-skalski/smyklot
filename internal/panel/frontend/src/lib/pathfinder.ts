/**
 * The finder's scorer: fzf-style in-order subsequence matching with affine
 * gaps and boundary, camel and consecutive bonuses. The index it runs over
 * is one deduped path list per workspace, shipped once - zero requests
 * per keystroke.
 */

export interface KnownPath {
  path: string;
  /** How many repositories already hold it. */
  repositories: number;
}

export interface PathMatch extends KnownPath {
  score: number;
  /** The matched character offsets, for the brand-ink marks. */
  positions: number[];
}

const isBoundary = (prev: string | undefined): boolean =>
  prev === '/' ||
  prev === '.' ||
  prev === '_' ||
  prev === '-' ||
  prev === ' ' ||
  prev === undefined;

/** One path against one query; null where the query is not a subsequence. */
export function fuzzyPath(
  query: string,
  path: string,
): { score: number; positions: number[] } | null {
  const q = query.toLowerCase();
  const p = path.toLowerCase();
  let qi = 0;
  let score = 0;
  let run = 0;
  let gapOpen = false;
  let started = false;
  const positions: number[] = [];
  for (let i = 0; i < p.length && qi < q.length; i += 1) {
    if (p[i] === q[qi]) {
      let bonus = 1;
      if (run > 0) bonus += 4 * Math.min(run, 4);
      const prev = path[i - 1];
      if (isBoundary(prev)) bonus += prev === '/' || prev === undefined ? 9 : 8;
      else if (/[a-z]/.test(prev ?? '') && /[A-Z]/.test(path[i] ?? '')) bonus += 7;
      if (qi === 0) bonus *= 2;
      score += bonus;
      positions.push(i);
      qi += 1;
      run += 1;
      gapOpen = false;
      started = true;
    } else if (started) {
      score -= gapOpen ? 1 : 3;
      gapOpen = true;
      run = 0;
    }
  }
  if (qi < q.length) return null;
  score -= Math.floor((path.length - (positions[positions.length - 1] ?? 0)) / 8);
  return { score, positions };
}

/**
 * The ranked suggestions for one query. An empty query ranks by how many
 * repositories hold each path - the popular paths are the likely asks.
 */
export function rankPaths(known: readonly KnownPath[], query: string, limit = 8): PathMatch[] {
  const ranked: PathMatch[] = [];
  for (const held of known) {
    const match =
      query === '' ? { score: held.repositories, positions: [] } : fuzzyPath(query, held.path);
    if (match !== null) ranked.push({ ...held, ...match });
  }
  ranked.sort((a, b) => b.score - a.score || a.path.length - b.path.length);
  return ranked.slice(0, limit);
}

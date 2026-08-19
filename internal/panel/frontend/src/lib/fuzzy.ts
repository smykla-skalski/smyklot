export interface FuzzyCandidate {
  id: string;
  label: string;
  keywords?: readonly string[];
}

export function fuzzyCandidates<T extends FuzzyCandidate>(
  candidates: readonly T[],
  rawQuery: string,
): T[] {
  const query = fold(rawQuery);
  if (query === '') return [...candidates];

  return candidates
    .map((candidate, index) => ({ candidate, index, score: candidateScore(candidate, query) }))
    .filter((entry) => entry.score !== null)
    .sort((left, right) => (left.score ?? 0) - (right.score ?? 0) || left.index - right.index)
    .map((entry) => entry.candidate);
}

function candidateScore(candidate: FuzzyCandidate, query: string): number | null {
  const values = [candidate.label, ...(candidate.keywords ?? [])].map(fold);
  let best: number | null = null;
  for (const value of values) {
    const score = fuzzyScore(value, query);
    if (score !== null && (best === null || score < best)) best = score;
  }
  return best;
}

function fuzzyScore(value: string, query: string): number | null {
  const exact = value.indexOf(query);
  if (exact >= 0) return exact * 2 + (value.length - query.length) * 0.01;

  let cursor = 0;
  let first = -1;
  let previous = -1;
  let gaps = 0;
  for (const character of query) {
    const found = value.indexOf(character, cursor);
    if (found < 0) return null;
    if (first < 0) first = found;
    if (previous >= 0) gaps += found - previous - 1;
    previous = found;
    cursor = found + 1;
  }
  return 100 + first * 2 + gaps + (value.length - query.length) * 0.01;
}

/**
 * A path the query matched, and where it matched, highest score first.
 *
 * `positions` is what the finder paints: the characters a reader typed, found
 * in the path they landed in. It comes from the same walk that produced the
 * score - a highlight worked out separately from the ranking is a highlight
 * that disagrees with it, which reads as the list being wrong.
 */
export interface PathMatch {
  path: string;
  score: number;
  positions: readonly number[];
}

/* fzf's own weights, which is where these numbers come from: a character is
   worth 16, a gap costs 3 to open and 1 to continue, and where a character sits
   is worth more than that it sits early - after a separator most, then a word
   boundary, then a hump in camelCase. The first character counts double,
   because a query is nearly always begun at the beginning of a name. */
const SCORE_MATCH = 16;
const GAP_START = -3;
const GAP_EXTEND = -1;
const BONUS_SEPARATOR = 9;
const BONUS_BOUNDARY = 8;
const BONUS_CAMEL = 7;
const BONUS_CONSECUTIVE = 4;
const FIRST_CHAR_MULTIPLIER = 2;
/** A whole band, not a nudge: it must outrank any run of bonuses below it. */
const BASENAME_BAND = 1000;

/**
 * Lowercased, and exactly as long as what went in.
 *
 * `positions` index this string and are then read against the original path -
 * by `bonusAt`, by `scoreOf`, and by the finder painting which characters
 * matched. `String.prototype.toLowerCase` does not promise to preserve length:
 * `'I'.toLowerCase()` is two code units (`i` + U+0307), so one Turkish-named
 * file made every index past it point one place too far, and a match near the
 * end read `path[length]`, which is `undefined` - and `undefined.toLowerCase()`
 * threw, taking the whole finder down for the installation that held it.
 *
 * A character whose lowercase is not one code unit keeps its own case. It then
 * fails a case-insensitive comparison it would otherwise have passed, which
 * costs one match on a rare character and is the trade this makes deliberately:
 * indices that line up are worth more here than folding every alphabet.
 */
function lower(value: string): string {
  let folded = '';
  for (const character of value) {
    const small = character.toLowerCase();
    folded += small.length === character.length ? small : character;
  }

  return folded;
}

/** What the character before this one says about where this one sits. */
function bonusAt(path: string, at: number): number {
  if (at === 0) return BONUS_SEPARATOR;
  const previous = path[at - 1];
  if (previous === '/') return BONUS_SEPARATOR;
  if (previous === '.' || previous === '-' || previous === '_' || previous === ' ') {
    return BONUS_BOUNDARY;
  }
  if (previous === lower(previous) && path[at] !== lower(path[at])) return BONUS_CAMEL;

  return 0;
}

/** One arrangement of the query inside the path, weighed as fzf weighs one. */
function scoreOf(path: string, query: string, positions: readonly number[]): number {
  let score = 0;
  let previous = -2;
  for (const [index, at] of positions.entries()) {
    const bonus = bonusAt(path, at);
    score += SCORE_MATCH;
    score += index === 0 ? bonus * FIRST_CHAR_MULTIPLIER : bonus;
    if (at === previous + 1) score += BONUS_CONSECUTIVE;
    else if (previous >= 0) score += GAP_START + GAP_EXTEND * (at - previous - 2);
    previous = at;
  }

  // A query with no separator in it is a file name, so a match that stays
  // inside the file name is a different kind of answer rather than a
  // better-scoring one. A query that does carry a separator has asked about
  // the whole path, and gets no band at all.
  if (!query.includes('/') && positions[0] > path.lastIndexOf('/')) score += BASENAME_BAND;

  return score;
}

/**
 * One path against one query, scored where it actually matched.
 *
 * Every place the query could begin is tried, and each is walked forward
 * greedily and then tightened backwards - which is what turns `store` matching
 * `internal/storage/sqlstore/store.go` into a run of five consecutive
 * characters in the file name rather than five letters picked out of the
 * directories above it. There are as many attempts as the first character has
 * occurrences, which for a path and a typed query is a handful.
 *
 * Full dynamic programming would find a marginally better arrangement in the
 * rare case where a later character, not the first, is the one worth moving -
 * and it costs a matrix per path per keystroke. This finds the arrangement a
 * reader would have pointed at, at a cost that survives twenty thousand paths.
 */
export function matchPath(path: string, query: string): PathMatch | null {
  if (query === '') return { path, score: 0, positions: [] };

  const haystack = lower(path);
  const needle = lower(query);

  let best: PathMatch | null = null;
  let from = haystack.indexOf(needle[0]);
  while (from >= 0) {
    let cursor = from;
    let complete = true;
    for (const character of needle) {
      const found = haystack.indexOf(character, cursor);
      if (found < 0) {
        complete = false;
        break;
      }
      cursor = found + 1;
    }
    // The first character has no later occurrence that completes the query,
    // and neither will any occurrence after it.
    if (!complete) break;

    // Backwards from where this attempt ended, each character taking the
    // latest position it can hold. Consecutive characters end up adjacent.
    const positions: number[] = [];
    let end = cursor - 1;
    for (let index = needle.length - 1; index >= 0; index -= 1) {
      const found = haystack.lastIndexOf(needle[index], end);
      positions.unshift(found);
      end = found - 1;
    }

    const score = scoreOf(path, query, positions);
    if (best === null || score > best.score) best = { path, score, positions };

    from = haystack.indexOf(needle[0], from + 1);
  }

  return best;
}

/**
 * Every path the query matches, best first, capped.
 *
 * The cap is the virtualisation: nobody reads past the tenth row of a finder,
 * and a list that stops at fifty never has to be windowed.
 */
export function matchPaths(paths: readonly string[], query: string, limit = 50): PathMatch[] {
  // Nothing typed is not a ranking question. The caller's order is the answer
  // - which is where a list of recent paths goes.
  if (query === '') {
    return paths.slice(0, limit).map((path) => ({ path, score: 0, positions: [] }));
  }

  const found: PathMatch[] = [];
  for (const path of paths) {
    const match = matchPath(path, query);
    if (match !== null) found.push(match);
  }

  return found
    .sort(
      (left, right) =>
        right.score - left.score ||
        left.path.length - right.path.length ||
        (left.path < right.path ? -1 : 1),
    )
    .slice(0, limit);
}

function fold(value: string): string {
  return value
    .trim()
    .normalize('NFKD')
    .replaceAll(/\p{Diacritic}/gu, '')
    .toLocaleLowerCase()
    .replaceAll('ł', 'l');
}

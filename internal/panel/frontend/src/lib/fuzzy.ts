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

function fold(value: string): string {
  return value
    .trim()
    .normalize('NFKD')
    .replaceAll(/\p{Diacritic}/gu, '')
    .toLocaleLowerCase()
    .replaceAll('ł', 'l');
}

/**
 * Line tokenizers for the small code windows the panel draws - a plan's file
 * diff, a template preview. Mock-grade on purpose: a handful of regular
 * expressions per language, because the panel shows excerpts, not an editor.
 *
 * Everything here returns data - runs of classed text - and never markup.
 * The component that renders a line owns the elements, so nothing a stored
 * document says can become HTML.
 */

export type CodeLang = 'json' | 'yaml' | 'markdown';

/** One stretch of a line wearing one token class. */
export interface TokenRun {
  /** A `tok-*` class, or undefined for plain text. */
  cls?: string;
  text: string;
  /** Inside a word-level diff emphasis - the changed stretch of a changed line. */
  word?: boolean;
}

const run = (text: string, cls?: string): TokenRun[] => (text === '' ? [] : [{ cls, text }]);

function tokenizeJson(line: string): TokenRun[] {
  const out: TokenRun[] = [];
  const re =
    /("(?:[^"\\]|\\.)*")(\s*:)?|(\/\/.*$)|(-?\d+(?:\.\d+)?)|\b(true|false|null)\b|([{}[\],:])/g;
  let last = 0;
  let match;
  while ((match = re.exec(line))) {
    out.push(...run(line.slice(last, match.index)));
    if (match[3] !== undefined) out.push(...run(match[3], 'tok-com'));
    else if (match[1] !== undefined && match[2] !== undefined) {
      out.push(...run(match[1], 'tok-key'), ...run(match[2], 'tok-pun'));
    } else if (match[1] !== undefined) out.push(...run(match[1], 'tok-str'));
    else if (match[4] !== undefined || match[5] !== undefined) {
      out.push(...run(match[4] ?? match[5] ?? '', 'tok-const'));
    } else if (match[6] !== undefined) out.push(...run(match[6], 'tok-pun'));
    last = re.lastIndex;
  }
  out.push(...run(line.slice(last)));
  return out;
}

function tokenizeYaml(line: string): TokenRun[] {
  const comment = /^(\s*)#(.*)$/.exec(line);
  if (comment !== null) {
    return [...run(comment[1] ?? ''), ...run(`#${comment[2] ?? ''}`, 'tok-com')];
  }
  const kv = /^(\s*(?:- )?)([\w./@-]+)(:)(.*)$/.exec(line);
  if (kv !== null) {
    const value = kv[4] ?? '';
    const constant = /^\s*-?\d+(\.\d+)?\s*$/.test(value) || /^\s*(true|false|null)\s*$/.test(value);
    return [
      ...run(kv[1] ?? '', 'tok-pun'),
      ...run(kv[2] ?? '', 'tok-key'),
      ...run(':', 'tok-pun'),
      ...run(value, constant ? 'tok-const' : 'tok-str'),
    ];
  }
  const item = /^(\s*- )(.*)$/.exec(line);
  if (item !== null) {
    return [...run(item[1] ?? '', 'tok-pun'), ...run(item[2] ?? '', 'tok-str')];
  }
  return run(line);
}

function tokenizeMarkdown(line: string): TokenRun[] {
  if (/^#{1,6} /.test(line)) return run(line, 'tok-head');
  const out: TokenRun[] = [];
  const listed = /^(- )(.*)$/.exec(line);
  let rest = line;
  if (listed !== null) {
    out.push(...run(listed[1] ?? '', 'tok-pun'));
    rest = listed[2] ?? '';
  }
  const re = /`[^`]+`/g;
  let last = 0;
  let match;
  while ((match = re.exec(rest))) {
    out.push(...run(rest.slice(last, match.index)));
    out.push(...run(match[0], 'tok-const'));
    last = re.lastIndex;
  }
  out.push(...run(rest.slice(last)));
  return out;
}

export function tokenizeLine(lang: CodeLang, line: string): TokenRun[] {
  if (lang === 'yaml') return tokenizeYaml(line);
  if (lang === 'markdown') return tokenizeMarkdown(line);
  return tokenizeJson(line);
}

/** [start, end) offsets of the emphasised stretch of a changed line. */
export type EmphasisRange = readonly [number, number];

/**
 * Re-cuts token runs so the stretches inside `ranges` carry `word: true`. Token
 * boundaries survive - a run straddling a range edge is split, never re-classed.
 */
export function emphasizeRuns(runs: TokenRun[], ranges: readonly EmphasisRange[]): TokenRun[] {
  if (ranges.length === 0) return runs;
  const inside = (at: number): boolean => ranges.some(([start, end]) => at >= start && at < end);
  const out: TokenRun[] = [];
  let offset = 0;
  for (const piece of runs) {
    let from = 0;
    while (from < piece.text.length) {
      const state = inside(offset + from);
      let to = from + 1;
      while (to < piece.text.length && inside(offset + to) === state) to += 1;
      out.push({
        ...(piece.cls === undefined ? {} : { cls: piece.cls }),
        text: piece.text.slice(from, to),
        ...(state ? { word: true } : {}),
      });
      from = to;
    }
    offset += piece.text.length;
  }
  return out;
}

/** One rendered line of a unified diff. */
export interface DiffLine {
  op: ' ' | '+' | '-';
  text: string;
  /** Word-level emphasis, present on the paired halves of a changed line. */
  emphasis?: EmphasisRange[];
}

/**
 * A unified diff of two small texts: longest-common-subsequence over lines,
 * deletions before the insertions that replace them. Where a deletion and an
 * insertion pair up, the changed stretch past their common prefix and suffix is
 * emphasised on both - the word inside the line that actually moved.
 */
export function unifiedDiff(before: string, after: string): DiffLine[] {
  const a = splitLines(before);
  const b = splitLines(after);

  /* LCS table - the texts here are excerpt-sized, so the quadratic table is
     smaller than the code any cleverer algorithm would take. */
  const table: number[][] = Array.from({ length: a.length + 1 }, () =>
    new Array<number>(b.length + 1).fill(0),
  );
  for (let i = a.length - 1; i >= 0; i -= 1) {
    for (let j = b.length - 1; j >= 0; j -= 1) {
      table[i]![j] =
        a[i] === b[j]
          ? (table[i + 1]![j + 1] ?? 0) + 1
          : Math.max(table[i + 1]![j] ?? 0, table[i]![j + 1] ?? 0);
    }
  }

  /* Walk the table: equal lines are context, and a changed region drains its
     deletions before the insertions that replace them, so a replacement reads
     as the old lines followed by the new ones. */
  const out: DiffLine[] = [];
  let i = 0;
  let j = 0;
  while (i < a.length || j < b.length) {
    if (i < a.length && j < b.length && a[i] === b[j]) {
      out.push({ op: ' ', text: a[i] ?? '' });
      i += 1;
      j += 1;
      continue;
    }
    const dels: string[] = [];
    const adds: string[] = [];
    while (
      i < a.length &&
      (j >= b.length || (a[i] !== b[j] && (table[i + 1]![j] ?? 0) >= (table[i]![j + 1] ?? 0)))
    ) {
      dels.push(a[i] ?? '');
      i += 1;
    }
    while (
      j < b.length &&
      (i >= a.length || (a[i] !== b[j] && (table[i + 1]![j] ?? 0) < (table[i]![j + 1] ?? 0)))
    ) {
      adds.push(b[j] ?? '');
      j += 1;
    }
    const paired = Math.min(dels.length, adds.length);
    dels.forEach((text, k) => {
      const line: DiffLine = { op: '-', text };
      const range = k < paired ? changedRange(text, adds[k] ?? '', 'before') : null;
      if (range !== null) line.emphasis = [range];
      out.push(line);
    });
    adds.forEach((text, k) => {
      const line: DiffLine = { op: '+', text };
      const range = k < paired ? changedRange(dels[k] ?? '', text, 'after') : null;
      if (range !== null) line.emphasis = [range];
      out.push(line);
    });
  }
  return out;
}

function splitLines(text: string): string[] {
  if (text === '') return [];
  return text.replace(/\n$/, '').split('\n');
}

/**
 * The stretch of one side of a paired change past the common prefix and
 * suffix of both - null where the lines are identical or nothing survives.
 */
function changedRange(
  before: string,
  after: string,
  side: 'before' | 'after',
): EmphasisRange | null {
  if (before === after) return null;
  let prefix = 0;
  const shortest = Math.min(before.length, after.length);
  while (prefix < shortest && before[prefix] === after[prefix]) prefix += 1;
  let suffix = 0;
  while (
    suffix < shortest - prefix &&
    before[before.length - 1 - suffix] === after[after.length - 1 - suffix]
  ) {
    suffix += 1;
  }
  const text = side === 'before' ? before : after;
  const end = text.length - suffix;
  if (end <= prefix) return null;
  return [prefix, end];
}

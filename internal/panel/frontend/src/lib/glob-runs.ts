/**
 * A pattern cut into the parts that MEAN something and the parts that are literal.
 *
 * A pattern is read to answer one question - what does this match - and the answer is
 * carried entirely by two or three characters inside a path that otherwise reads as a
 * path. `release/*` and `release/v2` are one glyph apart and one is a rule.
 *
 * The rule is the design system's: a `~TOKEN` colours whole, because the token IS the
 * meaning; a `*` colours itself, because the text around it is a literal.
 */
export interface GlobRun {
  text: string;
  meta: boolean;
}

export function globRuns(pattern: string): GlobRun[] {
  if (pattern.startsWith('~')) return [{ text: pattern, meta: true }];

  const runs: GlobRun[] = [];
  let literal = '';
  for (const character of pattern) {
    if (character === '*') {
      if (literal !== '') runs.push({ text: literal, meta: false });
      literal = '';
      const last = runs.at(-1);
      // `**` is one run, so it inks as the one token a reader reads it as.
      if (last?.meta === true) last.text += character;
      else runs.push({ text: character, meta: true });
      continue;
    }
    literal += character;
  }
  if (literal !== '') runs.push({ text: literal, meta: false });

  return runs;
}

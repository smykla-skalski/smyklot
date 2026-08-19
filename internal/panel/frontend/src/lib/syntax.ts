/**
 * Line-at-a-time colouring for the four languages a synced file is written in.
 *
 * Configuration is names and what they are set to, so the vocabulary is small
 * on purpose: a key, a value that is a word, a value that is a literal, a
 * comment, and the punctuation holding them together. Five roles carry JSON,
 * YAML, TOML and Markdown between them, and five is what the palette can space
 * far enough apart to stay legible under every dichromacy.
 *
 * A line is tokenised on its own, with no state carried from the line above.
 * That is a real limit - a multi-line string in YAML colours as though each of
 * its lines were a fresh one - and it is what makes the function total: a diff
 * hands over lines that never existed in one document together, and there is no
 * parse state that could span them.
 *
 * Tokens come back as data rather than markup. The caller renders them as
 * elements, so a value containing `<` is text and never a tag, and the diff can
 * cut a token in half where a changed word starts.
 */
export type TokenKind = 'plain' | 'key' | 'const' | 'string' | 'comment' | 'punct' | 'heading';

export type Language = 'json' | 'yaml' | 'toml' | 'markdown';

export interface Token {
  text: string;
  kind: TokenKind;
}

/** Drops the empties a split leaves behind, so a caller never renders one. */
function push(tokens: Token[], text: string, kind: TokenKind): void {
  if (text !== '') tokens.push({ text, kind });
}

const JSON_TOKEN =
  /("(?:[^"\\]|\\.)*")(\s*:)?|(\/\/.*$)|(-?\d+(?:\.\d+)?)|\b(true|false|null)\b|([{}[\],:])/g;

function json(line: string): Token[] {
  const tokens: Token[] = [];
  let last = 0;
  let match: RegExpExecArray | null;
  JSON_TOKEN.lastIndex = 0;
  while ((match = JSON_TOKEN.exec(line)) !== null) {
    push(tokens, line.slice(last, match.index), 'plain');
    const [, quoted, colon, comment, number, word, punctuation] = match;
    if (comment !== undefined) push(tokens, comment, 'comment');
    else if (quoted !== undefined && colon !== undefined) {
      push(tokens, quoted, 'key');
      push(tokens, colon, 'punct');
    } else if (quoted !== undefined) push(tokens, quoted, 'string');
    else if (number !== undefined) push(tokens, number, 'const');
    else if (word !== undefined) push(tokens, word, 'const');
    else if (punctuation !== undefined) push(tokens, punctuation, 'punct');
    last = JSON_TOKEN.lastIndex;
  }
  push(tokens, line.slice(last), 'plain');

  return tokens;
}

/** A bare scalar: `true`, `12`, `null` read as literals, anything else as text. */
function scalar(value: string): TokenKind {
  return /^\s*(-?\d+(\.\d+)?|true|false|null|yes|no|~)\s*$/i.test(value) ? 'const' : 'string';
}

function yaml(line: string): Token[] {
  const comment = /^(\s*)(#.*)$/.exec(line);
  if (comment !== null) return [{ text: comment[1] + comment[2], kind: 'comment' }];

  const pair = /^(\s*(?:- )?)([\w./@-]+)(:)(.*)$/.exec(line);
  if (pair !== null) {
    const tokens: Token[] = [];
    push(tokens, pair[1], 'punct');
    push(tokens, pair[2], 'key');
    push(tokens, pair[3], 'punct');
    push(tokens, pair[4], scalar(pair[4]));

    return tokens;
  }

  const item = /^(\s*- )(.*)$/.exec(line);
  if (item !== null) {
    const tokens: Token[] = [];
    push(tokens, item[1], 'punct');
    push(tokens, item[2], scalar(item[2]));

    return tokens;
  }

  return [{ text: line, kind: 'plain' }];
}

function toml(line: string): Token[] {
  const comment = /^(\s*)(#.*)$/.exec(line);
  if (comment !== null) return [{ text: comment[1] + comment[2], kind: 'comment' }];

  // A table header is the document's own structure, so it reads as a heading
  // rather than as one more key.
  const table = /^\s*\[.+]\s*$/.exec(line);
  if (table !== null) return [{ text: line, kind: 'heading' }];

  const pair = /^(\s*)([\w."-]+)(\s*=\s*)(.*)$/.exec(line);
  if (pair !== null) {
    const tokens: Token[] = [];
    push(tokens, pair[1], 'plain');
    push(tokens, pair[2], 'key');
    push(tokens, pair[3], 'punct');
    tokens.push(...json(pair[4]).map((token) => (token.kind === 'plain' ? { ...token } : token)));

    return tokens;
  }

  return [{ text: line, kind: 'plain' }];
}

function markdown(line: string): Token[] {
  if (/^#{1,6} /.test(line)) return [{ text: line, kind: 'heading' }];

  const tokens: Token[] = [];
  let rest = line;
  const bullet = /^([-*+] |\d+\. )/.exec(line);
  if (bullet !== null) {
    push(tokens, bullet[1], 'punct');
    rest = line.slice(bullet[1].length);
  }

  // Code spans only. Emphasis is left alone: a template is read for what it
  // says, and marking every asterisk turns prose into confetti.
  const code = /`[^`]*`/g;
  let last = 0;
  let match: RegExpExecArray | null;
  while ((match = code.exec(rest)) !== null) {
    push(tokens, rest.slice(last, match.index), 'plain');
    push(tokens, match[0], 'const');
    last = code.lastIndex;
  }
  push(tokens, rest.slice(last), 'plain');

  return tokens;
}

const LANGUAGES: Record<Language, (line: string) => Token[]> = { json, yaml, toml, markdown };

/** One line, coloured. An empty line comes back as an empty list. */
export function tokenize(line: string, language: Language): Token[] {
  return line === '' ? [] : LANGUAGES[language](line);
}

/**
 * The same line, cut where a changed range begins and ends.
 *
 * The file's own colouring is worked out first and the change marks are laid
 * over it, because the reverse - marking the words and then colouring what is
 * left - loses the colouring of any word that changed, which is the word most
 * worth reading.
 *
 * Ranges are half-open character offsets into the line, in the order the diff
 * produced them; overlapping or unsorted ranges are handled by asking each
 * piece whether its own start is inside any of them.
 */
export function tokenizeMarked(
  line: string,
  language: Language,
  marks: readonly (readonly [number, number])[],
): (Token & { marked: boolean })[] {
  const tokens = tokenize(line, language);
  if (marks.length === 0) return tokens.map((token) => ({ ...token, marked: false }));

  const cuts = new Set<number>();
  for (const [from, to] of marks) {
    cuts.add(from);
    cuts.add(to);
  }

  const marked = (at: number): boolean => marks.some(([from, to]) => at >= from && at < to);

  const out: (Token & { marked: boolean })[] = [];
  let at = 0;
  for (const token of tokens) {
    const end = at + token.text.length;
    const inside = [...cuts].filter((cut) => cut > at && cut < end).sort((a, b) => a - b);
    let from = at;
    for (const cut of [...inside, end]) {
      out.push({
        text: line.slice(from, cut),
        kind: token.kind,
        marked: marked(from),
      });
      from = cut;
    }
    at = end;
  }

  return out;
}

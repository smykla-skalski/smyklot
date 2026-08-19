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

/**
 * The lines already coloured, so typing does not re-colour the file.
 *
 * An editor rebuilds its whole line list on every keystroke - a new object per
 * line, so nothing downstream can tell which line actually changed - and every
 * one of them was run through the regular expressions again. A five hundred
 * line template meant five hundred colourings per key, of which one line's had
 * changed.
 *
 * Bounded, and cleared whole when it fills: a line's colouring is worth keeping
 * while a file is open and worth nothing afterwards, so the cheapest eviction
 * is the right one. Keyed by language as well, because the same text is not the
 * same tokens in YAML and in Markdown.
 */
const COLOURED = new Map<string, Token[]>();
const COLOURED_MAX = 4000;

/**
 * One line, coloured. An empty line comes back as an empty list.
 *
 * The answer is shared rather than copied. Nothing here writes to a token - the
 * components only read it - and copying every line on every read would give
 * back what the memo saves.
 */
export function tokenize(line: string, language: Language): Token[] {
  if (line === '') return [];

  const key = `${language}\u0000${line}`;
  const known = COLOURED.get(key);
  if (known !== undefined) return known;

  const tokens = LANGUAGES[language](line);
  if (COLOURED.size >= COLOURED_MAX) COLOURED.clear();
  COLOURED.set(key, tokens);

  return tokens;
}

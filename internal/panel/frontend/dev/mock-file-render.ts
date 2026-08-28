import { composeMergedText } from '../src/lib/jsontext.ts';
import { parseTree, visit, type Node, type ParseError } from 'jsonc-parser';
import {
  applyFormattingPatch,
  parseFormattingPatch,
  parseFormattingPolicy,
  type FormattingPolicy,
} from '../src/lib/formatting.ts';
import type {
  SyncFileMerge,
  SyncFileRenderInput,
  SyncFileRenderResponse,
} from '../src/lib/types.ts';

const SUPPORTED_PATH = /\.(?:jsonc?|ya?ml|toml|md|markdown)$/iu;

/**
 * Render the development server's file preview through the same typed wire
 * boundary as production.
 *
 * The browser's merge composer is intentionally only an editing aid. The mock
 * is the backend in development, so this module owns its deterministic output
 * and stays separate from the route plumbing and fixture construction.
 */
export function renderMockSyncFile(value: unknown): SyncFileRenderResponse {
  const input = parseInput(value);
  if (input === null) return invalid('invalid_request', 'the render request is invalid');
  if (!SUPPORTED_PATH.test(input.path)) return valid(input.draft_content, input.draft_content);

  let policy: FormattingPolicy = input.base_policy;
  for (const overlay of input.overlays ?? []) policy = applyFormattingPatch(policy, overlay);

  const substituted =
    input.default_branch === undefined || input.default_branch === ''
      ? input.draft_content
      : input.draft_content.replaceAll('{{DEFAULT_BRANCH}}', input.default_branch);
  const composed = compose(substituted, input.merge);
  if (composed === null) {
    return invalid(
      'invalid_document',
      'the development renderer cannot safely compose this file adjustment',
    );
  }
  const collections = formatCollections(composed, input.path, policy);
  if (collections.problem !== undefined) {
    return invalid('unsafe_formatting', collections.problem);
  }
  const formatted = formatCommon(collections.content, policy);
  return valid(input.draft_content, formatted);
}

type MockFormatResult = { content: string; problem?: never } | { content?: never; problem: string };

/**
 * Keep the development preview honest for the collection rule exercised by
 * the seeded repository-path example. This is deliberately a source-span
 * renderer, not JSON.parse plus JSON.stringify: values keep their spelling,
 * object layout, and comments outside the collection being changed.
 */
function formatCollections(
  content: string,
  path: string,
  policy: FormattingPolicy,
): MockFormatResult {
  if (!/\.jsonc?$/iu.test(path) || policy.json.arrays === 'preserve') return { content };

  const errors: ParseError[] = [];
  const root = parseTree(content, errors, { allowTrailingComma: true });
  if (root === undefined || errors.length > 0) {
    return { problem: 'the development renderer cannot read the JSON collection layout' };
  }

  try {
    return { content: renderJSONNode(content, root, policy) };
  } catch (cause) {
    return {
      problem:
        cause instanceof Error ? cause.message : 'the JSON collection cannot be formatted safely',
    };
  }
}

function renderJSONNode(content: string, node: Node, policy: FormattingPolicy): string {
  const children = node.children ?? [];
  const renderedChildren = children.map((child) => renderJSONNode(content, child, policy));
  const preserved = replaceChildren(content, node, children, renderedChildren);
  if (node.type !== 'array') return preserved;

  const mode = policy.json.arrays;
  const safeToReflow = !hasJSONComment(content.slice(node.offset, node.offset + node.length));
  const compact = `[${renderedChildren.join(', ')}]`;
  const oneLine = renderedChildren.every((child) => !/[\r\n]/u.test(child));
  if (mode === 'compact') {
    if (!safeToReflow || !oneLine) {
      throw new Error('compact JSON arrays cannot contain comments or multiline values');
    }
    return compact;
  }
  if (mode === 'auto') {
    if (!safeToReflow || !oneLine) return preserved;
    const column = sourceColumn(content, node.offset);
    return column + compact.length <= policy.common.line_width
      ? compact
      : expandedJSONArray(content, node, renderedChildren, policy);
  }
  if (mode === 'expanded') {
    if (!safeToReflow) throw new Error('expanded JSON arrays cannot move comments safely');
    return expandedJSONArray(content, node, renderedChildren, policy);
  }
  return preserved;
}

function replaceChildren(
  content: string,
  node: Node,
  children: readonly Node[],
  renderedChildren: readonly string[],
): string {
  let result = '';
  let cursor = node.offset;
  for (const [index, child] of children.entries()) {
    result += content.slice(cursor, child.offset);
    result += renderedChildren[index] ?? '';
    cursor = child.offset + child.length;
  }
  return result + content.slice(cursor, node.offset + node.length);
}

function expandedJSONArray(
  content: string,
  node: Node,
  children: readonly string[],
  policy: FormattingPolicy,
): string {
  if (children.length === 0) return '[]';
  const eol = dominantEOL(content);
  const parentIndent = sourceIndent(content, node.offset);
  const unit =
    policy.common.indent_style === 'tabs' ? '\t' : ' '.repeat(policy.common.indent_width);
  const childIndent = `${parentIndent}${unit}`;
  const entries = children.map((child) => child.replace(/\r?\n/gu, `${eol}${childIndent}`));
  return `[${eol}${childIndent}${entries.join(`,${eol}${childIndent}`)}${eol}${parentIndent}]`;
}

function hasJSONComment(content: string): boolean {
  let found = false;
  visit(content, {
    onComment: () => {
      found = true;
    },
  });
  return found;
}

function sourceColumn(content: string, offset: number): number {
  const lineStart = Math.max(
    content.lastIndexOf('\n', offset - 1),
    content.lastIndexOf('\r', offset - 1),
  );
  return offset - lineStart - 1;
}

function sourceIndent(content: string, offset: number): string {
  const lineStart = Math.max(
    content.lastIndexOf('\n', offset - 1),
    content.lastIndexOf('\r', offset - 1),
  );
  return /^[\t ]*/u.exec(content.slice(lineStart + 1, offset))?.[0] ?? '';
}

function parseInput(value: unknown): SyncFileRenderInput | null {
  if (!isRecord(value)) return null;
  if (typeof value.path !== 'string' || value.path.length === 0) return null;
  if (typeof value.draft_content !== 'string') return null;
  const basePolicy = parseFormattingPolicy(value.base_policy);
  if (basePolicy === null) return null;
  if (value.default_branch !== undefined && typeof value.default_branch !== 'string') return null;
  if (value.merge !== undefined && !isRecord(value.merge)) return null;
  if (value.overlays !== undefined && !Array.isArray(value.overlays)) return null;
  const overlays = (value.overlays ?? []).map(parseFormattingPatch);
  if (overlays.some((overlay) => overlay === null)) return null;
  return {
    path: value.path,
    draft_content: value.draft_content,
    base_policy: basePolicy,
    ...(value.default_branch === undefined ? {} : { default_branch: value.default_branch }),
    ...(value.merge === undefined ? {} : { merge: value.merge as Omit<SyncFileMerge, 'path'> }),
    ...(overlays.length === 0 ? {} : { overlays: overlays as NonNullable<(typeof overlays)[0]>[] }),
  };
}

function compose(content: string, merge: SyncFileRenderInput['merge']): string | null {
  if (merge === undefined || Object.keys(merge).length === 0) return content;
  if ((merge.strategy ?? 'deep-merge') === 'markdown') return composeMarkdown(content, merge);
  return composeMergedText(content, merge);
}

function composeMarkdown(content: string, merge: SyncFileRenderInput['merge']): string | null {
  if (!Array.isArray(merge?.sections)) return null;
  let result = content;
  for (const candidate of merge.sections) {
    if (!isRecord(candidate) || typeof candidate.action !== 'string') return null;
    const action = candidate.action;
    const text = typeof candidate.content === 'string' ? candidate.content : '';
    if (action === 'append') {
      result = joinBlocks(result, text);
      continue;
    }
    if (action === 'prepend') {
      result = joinBlocks(text, result);
      continue;
    }
    if (typeof candidate.heading !== 'string') return null;
    const span = markdownSectionSpan(result, candidate.heading, candidate.occurrence);
    if (span === null) return null;
    if (action === 'before') result = insertBlock(result, span.start, text);
    else if (action === 'after') result = insertBlock(result, span.end, text);
    else if (action === 'replace') result = replaceSpan(result, span.start, span.end, text);
    else if (action === 'delete') result = replaceSpan(result, span.start, span.end, '');
    else if (action === 'patch') {
      if (!Array.isArray(candidate.patches)) return null;
      let section = result.slice(span.start, span.end);
      for (const patch of candidate.patches) {
        if (
          !isRecord(patch) ||
          typeof patch.find !== 'string' ||
          typeof patch.replace !== 'string'
        ) {
          return null;
        }
        if (!section.includes(patch.find)) return null;
        section = section.replace(patch.find, patch.replace);
      }
      result = `${result.slice(0, span.start)}${section}${result.slice(span.end)}`;
    } else return null;
  }
  return result;
}

function markdownSectionSpan(
  content: string,
  wanted: string,
  occurrence: unknown,
): { start: number; end: number } | null {
  const heading = /^(#{1,6})[\t ]+(.+?)[\t ]*#*[\t ]*$/u.exec(wanted);
  if (heading === null) return null;
  const level = heading[1]!.length;
  const title = heading[2]!.toLocaleLowerCase();
  const matches: Array<{ start: number; level: number; title: string }> = [];
  const headings: Array<{ start: number; level: number; title: string }> = [];
  const pattern = /^(#{1,6})[\t ]+(.+?)[\t ]*#*[\t ]*(?:\r?\n|$)/gmu;
  for (const found of content.matchAll(pattern)) {
    const at = found.index;
    if (at === undefined) continue;
    const item = {
      start: at,
      level: found[1]!.length,
      title: found[2]!.toLocaleLowerCase(),
    };
    headings.push(item);
    if (item.level === level && item.title === title) matches.push(item);
  }
  const requested = typeof occurrence === 'number' && occurrence > 0 ? occurrence - 1 : 0;
  if (matches.length === 0 || requested >= matches.length) return null;
  if (occurrence === undefined && matches.length > 1) return null;
  const selected = matches[requested]!;
  const selectedIndex = headings.indexOf(selected);
  const next = headings.slice(selectedIndex + 1).find((candidate) => candidate.level <= level);
  return { start: selected.start, end: next?.start ?? content.length };
}

function replaceSpan(content: string, start: number, end: number, replacement: string): string {
  const before = content.slice(0, start).replace(/[\t ]*(?:\r?\n)*$/u, '');
  const after = content.slice(end).replace(/^(?:\r?\n)*[\t ]*/u, '');
  return [before, replacement.trim(), after].filter((part) => part !== '').join('\n\n');
}

function insertBlock(content: string, offset: number, block: string): string {
  const before = content.slice(0, offset);
  const rest = content.slice(offset);
  return joinBlocks(before, joinBlocks(block, rest));
}

function joinBlocks(left: string, right: string): string {
  if (left.trim() === '') return right.trim();
  if (right.trim() === '') return left.trim();
  return `${left.trimEnd()}\n\n${right.trimStart()}`;
}

function formatCommon(content: string, policy: FormattingPolicy): string {
  let rendered = content;
  if (policy.common.line_ending === 'lf') rendered = rendered.replaceAll('\r\n', '\n');
  if (policy.common.line_ending === 'crlf') {
    rendered = rendered.replaceAll('\r\n', '\n').replaceAll('\n', '\r\n');
  }
  const eol = policy.common.line_ending === 'crlf' ? '\r\n' : dominantEOL(rendered);
  if (policy.common.final_newline === 'insert' && !/(?:\r\n|\n)$/u.test(rendered)) {
    rendered += eol;
  }
  if (policy.common.final_newline === 'remove') rendered = rendered.replace(/(?:\r\n|\n)+$/u, '');
  return rendered;
}

function dominantEOL(content: string): '\n' | '\r\n' {
  return content.includes('\r\n') ? '\r\n' : '\n';
}

function valid(before: string, content: string): SyncFileRenderResponse {
  return { valid: true, content, changed: before !== content, diagnostics: [] };
}

function invalid(code: string, message: string): SyncFileRenderResponse {
  return { valid: false, content: '', changed: false, diagnostics: [{ code, message }] };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

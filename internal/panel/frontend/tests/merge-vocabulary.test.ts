import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import { ARRAY_STRATEGIES } from '#lib/merge.js';
import { fileFormat, isStructuredFile } from '#lib/file-format.js';

/**
 * The repository's Sync pane spells a merge the way the engine spells one.
 *
 * `filemerge` decides what a repository may do to a template, and the pane
 * restates that vocabulary in three segmented controls and one extension test.
 * Nothing between them checks the two agree, and the cost of drift is not a
 * cosmetic one: a `SectionAction` added to Go ships a panel that cannot express
 * it, and one removed leaves a control that refuses every save that chooses it.
 *
 * That is the bug this pane already had. It offered a path, a strategy and a
 * JSON object while the engine had implemented list rules, deduplication and
 * Markdown sections all along, and nothing failed - both halves compiled. So
 * the Go declaration is the vocabulary of record and this reads it, the way
 * `queue-vocabulary.test.ts` reads `pendingci/types.go`.
 */

const SPEC_SOURCE = new URL('../../../orgsync/filemerge/spec.go', import.meta.url);
const PANE_SOURCE = new URL('../src/lib/components/RepositorySyncPane.svelte', import.meta.url);
const RULES_SOURCE = new URL('../src/lib/components/StructuredMergeRules.svelte', import.meta.url);

/**
 * The values of one `const` block in `spec.go`, by the type they are typed as.
 *
 * Guarded on a floor, because this answers a question of the form "is anything
 * missing" and a parse that reads nothing is indistinguishable from agreement.
 */
function declared(type: string, floor: number): string[] {
  const source = readFileSync(SPEC_SOURCE, 'utf8');
  // The space matters: without it `Strategy` also matches `ArrayStrategy`.
  const found = [...source.matchAll(new RegExp(`\\s${type} = "(?<value>[a-z-]+)"`, 'gu'))].map(
    (match) => match.groups?.value ?? '',
  );

  if (found.length < floor) {
    throw new Error(`${type} parsed to only ${found.length} values, expected at least ${floor}`);
  }

  return found;
}

/** The values one option list offers, empty ones left out. */
function valuesOf(file: URL, list: string, floor: number): string[] {
  const source = readFileSync(file, 'utf8');
  const start = source.indexOf(`const ${list} = [`);

  if (start < 0) throw new Error(`${list} is no longer declared as an array literal`);

  // Whichever closes it first: one of these lists is `as const` and one is not,
  // and a search for the wrong closer runs on into the next declaration.
  const ends = ['] as const', '];']
    .map((closer) => source.indexOf(closer, start))
    .filter((at) => at > 0);
  const body = source.slice(start, Math.min(...ends));
  const found = [...body.matchAll(/value: '(?<value>[a-z-]*)'/gu)]
    .map((match) => match.groups?.value ?? '')
    .filter((value) => value !== '');

  if (found.length < floor) {
    throw new Error(`${list} parsed to only ${found.length} options, expected at least ${floor}`);
  }

  return found;
}

/** The values one option list in the pane offers. */
function offered(list: string, floor: number): string[] {
  return valuesOf(PANE_SOURCE, list, floor);
}

describe('merge vocabulary [Unit]', () => {
  /*
   * Split across two controls, because the engine refuses a Markdown strategy
   * on a structured file and a structured one on Markdown. Together they are
   * every strategy the engine has.
   */
  it('offers every strategy the engine has', () => {
    const both = [
      ...valuesOf(RULES_SOURCE, 'OBJECT_CHOICES', 2),
      ...offered('MARKDOWN_STRATEGIES', 1),
    ];

    expect(both.toSorted()).toEqual(declared('Strategy', 3).toSorted());
  });

  /*
   * Through `merge.ts` rather than pane-to-Go, because this list is now typed:
   * `SyncArrayRule.strategy` is `ArrayStrategy`, so a fourth word cannot reach
   * a stored rule without the compiler saying so. What is left to check is that
   * the shared list still matches Go, and that each place a reader CHOOSES from
   * offers all of it - a control missing `prepend` compiles perfectly.
   */
  it('shares one list strategy vocabulary with the engine', () => {
    expect([...ARRAY_STRATEGIES].toSorted()).toEqual(declared('ArrayStrategy', 3).toSorted());
  });

  it.each([
    ['LIST_CHOICES', RULES_SOURCE],
    ['RULE_CHOICES', new URL('../src/lib/components/SyncFilePage.svelte', import.meta.url)],
  ])('offers every list strategy in %s', (list, source) => {
    expect(valuesOf(source, list, 3).toSorted()).toEqual([...ARRAY_STRATEGIES].toSorted());
  });

  it('offers every section action the engine has', () => {
    expect(offered('SECTION_ACTIONS', 7).toSorted()).toEqual(
      declared('SectionAction', 7).toSorted(),
    );
  });

  it('shares all backend file capabilities with validation and the editor', () => {
    const source = readFileSync(SPEC_SOURCE, 'utf8');
    const extensionsOf = (functionName: string) => {
      const body = source.slice(source.indexOf(`func ${functionName}`));
      return [...body.slice(0, body.indexOf('\n}')).matchAll(/"\.(?<ext>\w+)"/gu)]
        .map((match) => match.groups?.ext ?? '')
        .toSorted();
    };
    const structured = extensionsOf('formatOf');
    const markdown = extensionsOf('isMarkdown');
    expect(structured.length).toBeGreaterThanOrEqual(5);
    expect(markdown.length).toBeGreaterThanOrEqual(2);
    for (const extension of structured) {
      expect(isStructuredFile(`config.${extension}`), extension).toBe(true);
      expect(isStructuredFile(`config.${extension.toUpperCase()}`), extension).toBe(true);
    }
    for (const extension of markdown) {
      expect(fileFormat(`guide.${extension}`)).toBe('markdown');
      expect(isStructuredFile(`guide.${extension}`)).toBe(false);
    }
    const mapping = readFileSync(new URL('../src/lib/file-format.ts', import.meta.url), 'utf8');
    const mapped = [...mapping.matchAll(/^ {2}(?<ext>\w+): '/gmu)]
      .map((match) => match.groups?.ext ?? '')
      .toSorted();
    expect(mapped).toEqual([...structured, ...markdown].toSorted());
    for (const path of ['LICENSE', 'config.ini', 'config.constructor', 'file.json/nested']) {
      expect(fileFormat(path)).toBeNull();
    }
  });
});

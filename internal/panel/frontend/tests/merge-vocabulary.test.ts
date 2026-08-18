import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

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

/** The values one option list in the pane offers, empty ones left out. */
function offered(list: string, floor: number): string[] {
  const source = readFileSync(PANE_SOURCE, 'utf8');
  const start = source.indexOf(`const ${list} = [`);

  if (start < 0) throw new Error(`${list} is no longer declared as an array literal`);

  const body = source.slice(start, source.indexOf('] as const', start));
  const found = [...body.matchAll(/value: '(?<value>[a-z-]*)'/gu)]
    .map((match) => match.groups?.value ?? '')
    .filter((value) => value !== '');

  if (found.length < floor) {
    throw new Error(`${list} parsed to only ${found.length} options, expected at least ${floor}`);
  }

  return found;
}

describe('merge vocabulary [Unit]', () => {
  /*
   * Split across two controls, because the engine refuses a Markdown strategy
   * on a structured file and a structured one on Markdown. Together they are
   * every strategy the engine has.
   */
  it('offers every strategy the engine has', () => {
    const both = [...offered('STRATEGIES', 2), ...offered('MARKDOWN_STRATEGIES', 1)];

    expect(both.toSorted()).toEqual(declared('Strategy', 3).toSorted());
  });

  it('offers every list strategy the engine has', () => {
    expect(offered('ARRAY_STRATEGIES', 3).toSorted()).toEqual(
      declared('ArrayStrategy', 3).toSorted(),
    );
  });

  it('offers every section action the engine has', () => {
    expect(offered('SECTION_ACTIONS', 7).toSorted()).toEqual(
      declared('SectionAction', 7).toSorted(),
    );
  });

  /*
   * The pane decides by regex which half of the form a row gets, and the engine
   * decides by a switch on the extension. A third extension added to one and
   * not the other silently sends a row to the wrong editor.
   */
  it('reads the same extensions as the engine', () => {
    const source = readFileSync(SPEC_SOURCE, 'utf8');
    const markdown = source.slice(source.indexOf('func isMarkdown'));
    const extensions = [...markdown.slice(0, markdown.indexOf('\n}')).matchAll(/"\.(?<ext>\w+)"/gu)]
      .map((match) => match.groups?.ext ?? '')
      .toSorted();

    expect(extensions.length).toBeGreaterThanOrEqual(2);

    const pane = readFileSync(PANE_SOURCE, 'utf8');
    const pattern = /const MARKDOWN_PATH = \/\\\.\(\?:(?<alternates>[a-z|]+)\)\$\/i;/u.exec(pane);

    if (pattern === null) throw new Error('MARKDOWN_PATH is no longer a literal alternation');

    expect((pattern.groups?.alternates ?? '').split('|').toSorted()).toEqual(extensions);
  });
});

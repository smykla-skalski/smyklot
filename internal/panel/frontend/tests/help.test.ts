import { describe, expect, it } from 'vitest';

import { HELP_COMMANDS, filterHelpCommands } from '../src/lib/help';

describe('help command search', () => {
  it('returns every command for an empty query', () => {
    expect(filterHelpCommands('')).toEqual(HELP_COMMANDS);
  });

  it('matches names, summaries, examples, and aliases without case sensitivity', () => {
    expect(filterHelpCommands('MERGE').map(({ name }) => name)).toEqual([
      'merge',
      'squash',
      'rebase',
    ]);
    expect(filterHelpCommands('/cleanup').map(({ name }) => name)).toEqual(['cleanup']);
    expect(filterHelpCommands('LGTM').map(({ name }) => name)).toEqual(['approve']);
  });

  it('returns no commands when nothing matches', () => {
    expect(filterHelpCommands('deploy')).toEqual([]);
  });
});

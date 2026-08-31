import { SYNC_KINDS, type RepositorySummary, type SyncStatus } from './types';

/**
 * What a repository says about itself, in the order a reader asks: whether it
 * is private, whether Smyklot answers there and where that was decided, what
 * its configuration file is doing, and what sync would change.
 *
 * One sentence for two surfaces. The list row and the repository's own page
 * used to say different things about the same repository - the row a real
 * account of it, the page a line of instructions that named nothing.
 */
export function repositorySentence(
  repository: RepositorySummary,
  on: boolean,
  fleet: SyncStatus | null = null,
  visibility = false,
): string {
  if (!repository.available) return 'Not reachable - Smyklot cannot see it';

  const parts: string[] = [];
  /* Only the page says it. The list has a column of names already narrow
     enough, and a word repeated down every row is a word nobody reads. */
  if (visibility) parts.push(repository.private ? 'Private' : 'Public');

  parts.push(on ? 'On' : 'Off');
  if (!on) {
    parts.push('commands are not answered here - sync still applies');
  } else if (repository.enabled_source === 'target') {
    parts.push('follows the workspace settings');
  } else if (repository.config_override_count > 0) {
    parts.push(
      `${repository.config_override_count} setting${
        repository.config_override_count === 1 ? '' : 's'
      } overridden here`,
    );
  } else {
    parts.push('switched on here');
  }

  if (repository.config_file_status === 'valid') parts.push('repository file followed');
  if (repository.config_file_status === 'invalid') parts.push('its file does not parse');
  if (repository.config_file_status === 'bypassed') parts.push('its file is bypassed');

  const sync = syncWord(repository.name, fleet);
  if (sync !== null) parts.push(sync);

  return parts.join(' · ');
}

/**
 * Where this repository stands with sync, or null when the fleet has not been
 * read. Silence rather than a guess: "not syncing" and "not asked yet" are
 * different facts, and a repository the fleet does not name is one the last
 * sweep did not reach rather than one that opted out.
 */
function syncWord(name: string, fleet: SyncStatus | null): string | null {
  if (fleet === null) return null;
  const row = fleet.repositories.find((entry) => entry.repository === name);
  if (row === undefined) return null;

  const cells = SYNC_KINDS.map((kind) => row.cells[kind]);
  if (cells.every((cell) => cell.state === 'off')) return 'sync off here';
  if (cells.some((cell) => cell.state === 'refused')) return 'sync refused here';

  const changes = cells.reduce((total, cell) => total + (cell.changes ?? 0), 0);
  if (changes === 0) return 'syncing · in step';

  return `syncing · ${changes} change${changes === 1 ? '' : 's'} in the open plan`;
}

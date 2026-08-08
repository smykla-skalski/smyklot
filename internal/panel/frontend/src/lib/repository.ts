import { PanelApiError } from './api';

export type RepositoryFailureSource = 'read' | 'write';

export function shouldReloadRepositoryAfterSaveFailure(error: unknown): boolean {
  return error instanceof PanelApiError && error.status === 409;
}

export function shouldClearFailureAfterAutomaticRefresh(
  source: RepositoryFailureSource | undefined,
): boolean {
  return source === 'read';
}

export function shouldReplaceFailureWithReadError(
  source: RepositoryFailureSource | undefined,
): boolean {
  return source !== 'write';
}

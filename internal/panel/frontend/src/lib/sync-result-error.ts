import { PanelApiError } from './api';

/** Recovery describes the failed read. It never starts another check or applies changes. */
export function syncResultProblem(error: unknown): {
  title: string;
  description: string;
  retry: boolean;
} {
  if (error instanceof PanelApiError) {
    if (error.status === 401)
      return {
        title: 'Sign in to view these changes',
        description: 'Your session could not be verified. Sign in again, then reopen this result',
        retry: false,
      };
    if (error.status === 403)
      return {
        title: 'You cannot view these changes',
        description:
          'Ask a workspace administrator to review your access before reopening this result',
        retry: false,
      };
    if (error.status === 404)
      return {
        title: 'These changes are unavailable',
        description:
          'This saved result could not be found. Returning keeps your place without starting new work',
        retry: false,
      };
  }
  return {
    title: 'Changes could not be loaded',
    description:
      'Try loading this result again. This only reads the saved changes and does not run sync',
    retry: true,
  };
}

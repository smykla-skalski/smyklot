import { SYNC_SECTION_LABELS } from './routes';
import { SYNC_KINDS, type SyncAction } from './types';

/** A recorded outcome must remain identifiable when its optional detail is missing. */
export function syncActionFeedback(action: SyncAction): string | null {
  const error = action.error?.trim();
  const blocker = action.blocker?.trim();
  if (action.state === 'failed') return `Failed: ${error || 'No failure details were recorded.'}`;
  if (action.state === 'skipped') {
    if (!blocker) return `Skipped: ${error || 'No reason was recorded.'}`;
    const dependency = SYNC_KINDS.find((kind) => kind === blocker);
    const kind = dependency ? SYNC_SECTION_LABELS[dependency] : undefined;
    return kind
      ? `Skipped because ${kind.toLowerCase()} failed earlier in this repository.`
      : `Skipped: ${blocker}`;
  }
  if (error || blocker) return error || blocker || null;
  if (action.state === 'applied') {
    if (action.kind === 'files')
      return action.proposal_url?.trim()
        ? 'Proposed in a pull request'
        : 'Completed; no pull request link recorded';
    return 'Succeeded';
  }
  return 'Pending';
}

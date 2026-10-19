import { navigating } from '$app/state';
import { fileFormat } from './file-format';

const PREFIX = '#file-sync=';

/** The repository remains one page; the fragment identifies an editor on it. */
export function fileAdjustmentHref(repositoryHref: string, path: string): string {
  return `${repositoryHref}${PREFIX}${encodeURIComponent(path)}`;
}

export function fileAdjustmentPath(hash: string): string | null {
  if (!hash.startsWith(PREFIX)) return null;
  try {
    const path = decodeURIComponent(hash.slice(PREFIX.length));
    return fileFormat(path) !== null &&
      !path.startsWith('/') &&
      !path.includes('\\') &&
      !path.split('/').some((part) => part === '' || part === '.' || part === '..')
      ? path
      : null;
  } catch {
    return null;
  }
}

/** Async repository reads can finish after the router's initial fragment scroll. */
export function revealFileAdjustment(node: HTMLElement): () => void {
  let cancelled = false;
  let frame: number | undefined;
  const schedule = () => {
    if (cancelled) return;
    frame = requestAnimationFrame(() => {
      if (!node.isConnected) return;
      const top = Math.max(
        0,
        document.querySelector('.top-bar')?.getBoundingClientRect().bottom ?? 0,
      );
      const gap = Number.parseFloat(getComputedStyle(node).scrollMarginBlockStart) || 0;
      window.scrollBy({ top: node.getBoundingClientRect().top - top - gap, behavior: 'instant' });
      node.focus({ preventScroll: true });
    });
  };
  // The router resets focus after the new component mounts.
  const complete = navigating.complete;
  if (complete) void complete.then(schedule, () => {});
  else schedule();
  return () => {
    cancelled = true;
    if (frame !== undefined) cancelAnimationFrame(frame);
  };
}

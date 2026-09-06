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
  const frame = requestAnimationFrame(() => {
    if (!node.isConnected) return;
    const top = Math.max(
      0,
      document.querySelector('.top-bar')?.getBoundingClientRect().bottom ?? 0,
    );
    const gap = Number.parseFloat(getComputedStyle(node).scrollMarginBlockStart) || 0;
    window.scrollBy({ top: node.getBoundingClientRect().top - top - gap, behavior: 'instant' });
    node.focus({ preventScroll: true });
  });
  return () => cancelAnimationFrame(frame);
}

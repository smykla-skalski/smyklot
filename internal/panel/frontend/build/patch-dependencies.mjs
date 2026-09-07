import { readFile, writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

// Bits 2.19.0 can reset a newly registered layer between an outside pointerdown
// and its debounced handler. Cancel the obsolete cleanup on ref/enable changes.
// Remove this patch once the pinned upstream release handles that interleaving.
const before = `            clearPendingTimer();
            pendingTimer = afterSleep(1, () => {`;
const after = `            clearPendingTimer();
            this.#resetState.destroy();
            pendingTimer = afterSleep(1, () => {`;

/** @param {string} frontend */
export async function patchDependencies(frontend) {
  const dependency = join(frontend, 'node_modules/bits-ui');
  const { version } = JSON.parse(await readFile(join(dependency, 'package.json'), 'utf8'));
  if (version !== '2.19.0') {
    throw new Error(`Review the Bits dismissal patch before using bits-ui ${version}`);
  }
  const path = join(
    dependency,
    'dist/bits/utilities/dismissible-layer/use-dismissable-layer.svelte.js',
  );
  const source = await readFile(path, 'utf8');
  if (source.split(after).length === 2 && !source.includes(before)) return;
  if (source.split(before).length !== 2 || source.includes(after)) {
    throw new Error('Bits dismissal source changed; review the dependency patch');
  }
  await writeFile(path, source.replace(before, after));
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  await patchDependencies(resolve(dirname(fileURLToPath(import.meta.url)), '..'));
}

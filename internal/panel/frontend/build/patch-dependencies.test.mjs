import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { patchDependencies } from './patch-dependencies.mjs';

const original = `            clearPendingTimer();
            pendingTimer = afterSleep(1, () => {`;

/** @param {import('node:test').TestContext} t */
async function fixture(t, version = '2.19.0', source = original) {
  const root = await mkdtemp(join(tmpdir(), 'smyklot-dependency-patch-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const dependency = join(root, 'node_modules/bits-ui');
  const target = join(
    dependency,
    'dist/bits/utilities/dismissible-layer/use-dismissable-layer.svelte.js',
  );
  await mkdir(join(dependency, 'dist/bits/utilities/dismissible-layer'), { recursive: true });
  await writeFile(join(dependency, 'package.json'), JSON.stringify({ version }));
  await writeFile(target, source);
  return { root, target };
}

test('fresh and cached installs cancel stale dismissal cleanup once', async (t) => {
  const { root, target } = await fixture(t);
  await patchDependencies(root);
  const patched = await readFile(target, 'utf8');
  assert.equal(
    patched,
    original.replace(
      '            pendingTimer',
      '            this.#resetState.destroy();\n            pendingTimer',
    ),
  );
  await patchDependencies(root);
  assert.equal(await readFile(target, 'utf8'), patched);
});

test('dependency upgrades require explicit patch review', async (t) => {
  const { root, target } = await fixture(t, '2.20.0');
  await assert.rejects(patchDependencies(root), /Review the Bits dismissal patch/);
  assert.equal(await readFile(target, 'utf8'), original);
});

test('changed or ambiguous source is never silently patched', async (t) => {
  for (const source of ['different implementation', original + original]) {
    const { root, target } = await fixture(t, '2.19.0', source);
    await assert.rejects(patchDependencies(root), /Bits dismissal source changed/);
    assert.equal(await readFile(target, 'utf8'), source);
  }
});

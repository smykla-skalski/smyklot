import { readFileSync, readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import type { Plugin } from 'vite';

/**
 * Each component's contract, read out of the component itself.
 *
 * A contract is the `<!-- @component -->` block Svelte already defines for this
 * purpose: an editor shows it on hover, `svelte-check` carries it, and this makes
 * the catalogue show it too. Written once, in the file it describes.
 *
 * It arrives through a virtual module rather than a checked-in generated one
 * because a generated file is a second copy that can be stale, and the whole reason
 * `tokens.css` exists is that a second copy of the design system had drifted from
 * the first. Storybook's docgen exposes `__docgen`, which carries the props and no
 * component description, so this fills the gap `preview.ts` reads through
 * `extractComponentDescription`.
 */
const VIRTUAL = 'virtual:component-contracts';
const RESOLVED = `\0${VIRTUAL}`;

const components = fileURLToPath(new URL('../src/lib/components/', import.meta.url));

/** The `<!-- @component -->` block's prose, with the marker and the fence removed. */
export function contractOf(source: string): string | undefined {
  const block = /<!--\s*@component\s*\n(?<body>[\s\S]*?)\n?-->/u.exec(source);
  const body = block?.groups?.body?.trim();
  return body === undefined || body === '' ? undefined : body;
}

/** Every component that carries one, by component name. */
export function contracts(): Record<string, string> {
  const found: Record<string, string> = {};
  for (const file of readdirSync(components)) {
    if (!file.endsWith('.svelte')) continue;
    const contract = contractOf(readFileSync(`${components}${file}`, 'utf8'));
    if (contract !== undefined) found[file.replace('.svelte', '')] = contract;
  }
  return found;
}

export function componentContracts(): Plugin {
  return {
    name: 'smyklot:component-contracts',
    resolveId: (id) => (id === VIRTUAL ? RESOLVED : undefined),
    load: (id) => (id === RESOLVED ? `export default ${JSON.stringify(contracts())};` : undefined),
    // A contract is edited in the component, so the module built from those files has
    // to be rebuilt when one changes - otherwise the catalogue shows the contract the
    // dev server started with until somebody restarts it.
    handleHotUpdate({ file, server }) {
      if (!file.startsWith(components) || !file.endsWith('.svelte')) return;
      const module = server.moduleGraph.getModuleById(RESOLVED);
      if (module !== undefined && module !== null) server.reloadModule(module);
    },
  };
}

import { parseTree } from 'jsonc-parser';
import { hasDuplicateKeys, preserveNumberToken } from '../src/lib/merge.js';

/** Match fields decoded by strict custom JSON unmarshallers in the service. */
export function parseRequestJSON(text: string, uniqueFields: readonly string[] = []): unknown {
  const value: unknown = JSON.parse(text, preserveNumberToken);
  if (uniqueFields.length === 0) return value;
  const root = parseTree(text);
  if (root?.type === 'object') {
    for (const property of root.children ?? []) {
      const [key, child] = property.children ?? [];
      if (uniqueFields.includes(key?.value as string) && child && hasDuplicateKeys(child)) {
        throw new SyntaxError('request field contains duplicate JSON properties');
      }
    }
  }
  return value;
}

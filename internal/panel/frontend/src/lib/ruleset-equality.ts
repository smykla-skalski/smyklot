import { formatJson, numericValue, parseJson, type JsonValue } from './merge';
import type { SyncRuleset } from './types';

const SET_PATHS = [
  ['conditions', 'include'],
  ['conditions', 'exclude'],
  ['bypass_actors'],
  ['rules', 'pull_request', 'allowed_merge_methods'],
  ['rules', 'required_status_checks', 'required_status_checks'],
  ['rules', 'code_scanning', 'code_scanning_tools'],
] as const;

/** Mirror orgsync.canonicalRuleset only for comparison, preserving authored order. */
export function sameRulesetDocument(left: string, right: string): boolean {
  const first = canonical(left);
  return first !== null && first === canonical(right);
}

/** Compare a ruleset or selected fields using the same semantics as the saved document. */
export function sameRulesetFields(
  left: Partial<SyncRuleset> | null,
  right: Partial<SyncRuleset> | null,
): boolean {
  // These API records contain JSON values; their interfaces intentionally do not
  // expose a string index signature. Keep the lossless number serializer here.
  return sameRulesetDocument(
    formatJson({ rulesets: [left ?? {}] } as unknown as JsonValue),
    formatJson({ rulesets: [right ?? {}] } as unknown as JsonValue),
  );
}

function canonical(text: string): string | null {
  const document = parseJson(text);
  if (!record(document) || !Array.isArray(document.rulesets)) return null;
  for (const ruleset of document.rulesets) {
    if (!record(ruleset)) return null;
    const rules = ruleset.rules;
    if (
      record(rules) &&
      record(rules.pull_request) &&
      numericValue(rules.pull_request.required_approving_review_count) === 0
    ) {
      delete rules.pull_request.required_approving_review_count;
    }
    if (Array.isArray(ruleset.bypass_actors)) {
      for (const actor of ruleset.bypass_actors) {
        if (
          record(actor) &&
          ['OrganizationAdmin', 'DeployKey'].includes(String(actor.actor_type))
        ) {
          actor.actor_id = 0;
        }
      }
    }
    for (const path of SET_PATHS) {
      let owner: Record<string, JsonValue> | null = ruleset;
      for (const key of path.slice(0, -1)) {
        const next: JsonValue | undefined = owner?.[key];
        owner = record(next) ? next : null;
      }
      if (owner === null) continue;
      const key = path.at(-1)!;
      const values = owner[key];
      if (values === null || (Array.isArray(values) && values.length === 0)) {
        delete owner[key];
      } else if (Array.isArray(values)) {
        owner[key] = values.toSorted((a, b) => {
          const first = formatJson(a);
          const second = formatJson(b);
          return first < second ? -1 : first > second ? 1 : 0;
        });
      }
    }
  }
  return formatJson(document);
}

function record(value: JsonValue | undefined): value is Record<string, JsonValue> {
  return (
    value !== undefined && value !== null && typeof value === 'object' && !Array.isArray(value)
  );
}

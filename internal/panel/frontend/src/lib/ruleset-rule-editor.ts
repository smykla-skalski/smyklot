import { formatJson, type JsonValue } from './merge';

export type EditableRuleKey =
  'pull_request' | 'required_status_checks' | 'update' | 'code_scanning';
export type RuleParameters = Record<string, unknown>;

export const ALERT_LEVELS = [
  { value: 'none', label: 'None' },
  { value: 'errors', label: 'Errors' },
  { value: 'errors_and_warnings', label: 'Errors and warnings' },
  { value: 'all', label: 'All' },
] as const;
export const SECURITY_LEVELS = [
  { value: 'none', label: 'None' },
  { value: 'critical', label: 'Critical' },
  { value: 'high_or_higher', label: 'High or higher' },
  { value: 'medium_or_higher', label: 'Medium or higher' },
  { value: 'all', label: 'All' },
] as const;

export function newRuleParameters(key: EditableRuleKey): RuleParameters {
  if (key === 'pull_request')
    return {
      required_approving_review_count: 1,
      allowed_merge_methods: ['merge', 'squash', 'rebase'],
    };
  if (key === 'required_status_checks')
    return { required_status_checks: [], strict_required_status_checks_policy: true };
  if (key === 'code_scanning') return { code_scanning_tools: [] };
  return {};
}

function sameValue(left: unknown, right: unknown): boolean {
  return (
    left === right ||
    (left !== undefined &&
      right !== undefined &&
      formatJson(left as JsonValue) === formatJson(right as JsonValue))
  );
}

/** Restore authored presence and literal bytes when a control returns to its opening value. */
export function withRuleField(
  original: RuleParameters,
  current: RuleParameters,
  field: string,
  value: unknown,
  openingValue: unknown = original[field],
): RuleParameters {
  const next = { ...current };
  if (sameValue(value, openingValue)) {
    if (Object.hasOwn(original, field)) next[field] = original[field];
    else delete next[field];
  } else next[field] = value;
  return next;
}

export function ruleItems(parameters: RuleParameters, field: string): RuleParameters[] {
  const value = parameters[field];
  return Array.isArray(value) ? (value as RuleParameters[]) : [];
}

/** Re-adding an opening entry retains its app pin, thresholds, unknown fields and order. */
export function addRuleItem(
  original: RuleParameters,
  current: RuleParameters,
  field: string,
  nameField: string,
  name: string,
  defaults: RuleParameters = {},
): RuleParameters {
  const held = ruleItems(current, field);
  if (held.some((item) => item[nameField] === name)) return current;
  const opening = ruleItems(original, field);
  const originalIndex = opening.findIndex((item) => item[nameField] === name);
  const item = originalIndex < 0 ? { ...defaults, [nameField]: name } : opening[originalIndex]!;
  const next = [...held];
  const nextOriginal =
    originalIndex < 0
      ? undefined
      : opening
          .slice(originalIndex + 1)
          .find((candidate) => held.some((entry) => entry[nameField] === candidate[nameField]));
  const position = nextOriginal
    ? held.findIndex((entry) => entry[nameField] === nextOriginal[nameField])
    : held.length;
  next.splice(position, 0, item);
  return withRuleField(original, current, field, next);
}

export function withMergeMethod(
  original: RuleParameters,
  current: RuleParameters,
  method: string,
): RuleParameters {
  const opening = (original.allowed_merge_methods as string[] | undefined) ?? [
    'merge',
    'squash',
    'rebase',
  ];
  const selected = (current.allowed_merge_methods as string[] | undefined) ?? opening;
  const next = selected.includes(method)
    ? selected.filter((value) => value !== method)
    : [...selected, method];
  const restored = next.length === opening.length && opening.every((value) => next.includes(value));
  return withRuleField(
    original,
    current,
    'allowed_merge_methods',
    restored ? opening : next,
    opening,
  );
}

import { describe, expect, it } from 'vitest';
import { formatJson, parseJson, type JsonValue } from '../src/lib/merge';
import {
  addRuleItem,
  withRuleField,
  withMergeMethod,
  type RuleParameters,
  ALERT_LEVELS,
  SECURITY_LEVELS,
} from '../src/lib/ruleset-rule-editor';
import { readFileSync } from 'node:fs';

const record = (text: string) => parseJson(text) as RuleParameters;
const wire = (value: unknown) => formatJson(value as JsonValue);

describe('rule parameter editing [Unit]', () => {
  it('changes one boolean while preserving every unknown and exact numeric field', () => {
    const original = record(
      '{"strict_required_status_checks_policy":false,"future":1e400,"required_status_checks":[{"context":"test","integration_id":9007199254740993,"extra":{"tiny":1e-400}}]}',
    );
    const before = wire(original);
    const changed = withRuleField(
      original,
      original,
      'strict_required_status_checks_policy',
      true,
      false,
    );
    expect(wire(changed)).toContain('9007199254740993');
    expect(wire(changed)).toContain('1e400');
    expect(wire(changed)).toContain('1e-400');
    expect(changed.required_status_checks).toBe(original.required_status_checks);
    expect(wire(original)).toBe(before);
    expect(
      wire(withRuleField(original, changed, 'strict_required_status_checks_policy', false, false)),
    ).toBe(before);
  });
  it('restores absent and authored numeric fields after control roundtrips', () => {
    for (const original of [
      record('{}'),
      record('{"required_approving_review_count":-0}'),
      record('{"required_approving_review_count":0.0}'),
    ]) {
      const changed = withRuleField(original, original, 'required_approving_review_count', 3, 0);
      expect(wire(withRuleField(original, changed, 'required_approving_review_count', 0, 0))).toBe(
        wire(original),
      );
    }
  });
  it('restores removed pinned checks in their original order with all child fields', () => {
    const original = record(
      '{"required_status_checks":[{"context":"test","integration_id":9007199254740993,"future":-0},{"context":"lint","integration_id":77}],"future":true}',
    );
    const checks = original.required_status_checks as RuleParameters[];
    const removed = withRuleField(original, original, 'required_status_checks', [checks[1]]);
    const restored = addRuleItem(original, removed, 'required_status_checks', 'context', 'test');
    expect(wire(restored)).toBe(wire(original));
    expect(addRuleItem(original, restored, 'required_status_checks', 'context', 'test')).toBe(
      restored,
    );
  });
  it('preserves scanning thresholds and child data when a tool is re-added', () => {
    const original = record(
      '{"code_scanning_tools":[{"tool":"CodeQL","alerts_threshold":"all","security_alerts_threshold":"critical","future":9007199254740993}]}',
    );
    const removed = withRuleField(original, original, 'code_scanning_tools', []);
    expect(
      wire(
        addRuleItem(original, removed, 'code_scanning_tools', 'tool', 'CodeQL', {
          alerts_threshold: 'errors',
          security_alerts_threshold: 'high_or_higher',
        }),
      ),
    ).toBe(wire(original));
  });
  it('restores authored merge-method order without dropping unrepresented methods', () => {
    const original = record(
      '{"allowed_merge_methods":["squash","merge","future-method"],"future":-0}',
    );
    const changed = withMergeMethod(original, original, 'squash');
    expect(changed.allowed_merge_methods).toContain('future-method');
    expect(wire(withMergeMethod(original, changed, 'squash'))).toBe(wire(original));
  });
  it('uses the exact alert thresholds accepted by the backend validator', () => {
    const source = readFileSync('../../orgsync/rulesets.go', 'utf8');
    for (const [marker, choices] of [
      ['alertsThresholds', ALERT_LEVELS],
      ['securityAlertsThresholds', SECURITY_LEVELS],
    ] as const) {
      const block = source.split(`${marker} = map[string]bool{`)[1]!.split('}')[0]!;
      const accepted = [...block.matchAll(/"([a-z_]+)": true/gu)].map((match) => match[1]);
      expect(choices.map((choice) => choice.value).sort()).toEqual(accepted.sort());
    }
  });
});

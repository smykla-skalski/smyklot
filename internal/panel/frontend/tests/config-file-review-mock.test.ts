import { describe, expect, it } from 'vitest';
import { mockConfigFilePreview } from '../dev/config-file-review';
import { formatJson } from '../src/lib/merge';

describe('configuration-file review fixture contract [Unit]', () => {
  it.each([false, true])(
    'preserves independent values and exact content for workspace=%s',
    (workspace) => {
      const preview = mockConfigFilePreview('conflict', 'acme/widget', 'a'.repeat(64), workspace);
      expect(preview.choices).toHaveLength(2);
      expect(preview.review_token).toHaveLength(64);
      for (const choice of preview.choices!) {
        const document = formatJson(choice.document!);
        expect(document).toContain('9007199254740993');
        expect(document).toContain('1e-400');
        expect(document).toContain('"quiet_success": false');
        expect(document).toContain('"allow_self_approval": true');
        expect(document).toContain(`"scope": "${workspace ? 'workspace' : 'repository'}"`);
        expect(choice.import_panel && choice.publish_file).toBe(true);
      }
    },
  );
  it.each(['invalid', 'schema', 'scope', 'missingAccess', 'unavailable', 'outstanding'] as const)(
    'does not manufacture choices for %s',
    (variant) => {
      const preview = mockConfigFilePreview(variant, 'acme/widget', 'a'.repeat(64));
      expect(preview.status).toBe('blocked');
      expect(preview.review_token).toBeUndefined();
      expect(preview.choices).toBeUndefined();
    },
  );
  it('distinguishes first creation from explicit recreation after deletion', () => {
    const initial = mockConfigFilePreview('missing', 'acme/widget', 'a'.repeat(64));
    expect(initial.status).toBe('pending');
    expect(initial.choices).toBeUndefined();
    const removed = mockConfigFilePreview('removed', 'acme/widget', 'a'.repeat(64));
    expect(removed.choices?.map((choice) => choice.side)).toEqual(['panel']);
    expect(removed.choices?.[0]?.import_panel).toBe(false);
  });
});

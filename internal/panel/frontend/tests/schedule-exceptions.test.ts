import { describe, expect, it } from 'vitest';
import {
  editableExceptions,
  exceptionDraft,
  exceptionInputs,
} from '../src/lib/schedule-exceptions';
import { scheduleExceptionProblems } from '../src/lib/schedule-validation';

describe('structured exception drafts [Unit]', () => {
  it('round-trips closed, partial and full-day exceptions without local ids', () => {
    const values = [
      { date: '2026-12-25', closed: true },
      { date: '2026-12-31', closed: false, start_minute: 540, end_minute: 780 },
      { date: '2027-01-01', closed: false, start_minute: 0, end_minute: 1440 },
    ];
    expect(exceptionInputs(editableExceptions(values))).toEqual(values);
  });
  it('retains incomplete input in snapshots but prevents it from being valid', () => {
    const values = [{ id: 'draft', date: '', closed: false, start: '', end: '12:00' }];
    expect(exceptionDraft(values)).toEqual([{ date: '', closed: false, start: '', end: '12:00' }]);
    expect(scheduleExceptionProblems(exceptionInputs(values))).toHaveLength(2);
  });
  it('does not dirty a draft when only local row identities change', () => {
    const values = editableExceptions([{ date: '2026-12-25', closed: true }]);
    expect(exceptionDraft(values)).toEqual(
      exceptionDraft(values.map((entry) => ({ ...entry, id: 'new-local-id' }))),
    );
  });
});

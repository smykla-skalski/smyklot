/**
 * The two words a sync form puts on a yes-or-no.
 *
 * Three forms on that page ask one, and each had written the pair out for
 * itself. That is the shape the list helpers beside this were pulled out of
 * for: the third copy is where a fourth quietly says something else, and a
 * switch reading "On / Off" beside one reading "Enabled / Disabled" is a
 * difference nobody chose.
 *
 * Not the page's own enablement switch, which says Enabled and Disabled and
 * lives in SyncDocumentForm. These are the switches inside a form, where the
 * subject is already named by the text beside them.
 */

/** What the stored value is, which is a string because SegmentedControl's is. */
export const ON = 'on';
export const OFF = 'off';

/** One choice, matching what SegmentedControl takes. */
export interface SwitchChoice {
  value: string;
  label: string;
}

export const SWITCH: readonly SwitchChoice[] = [
  { value: ON, label: 'On' },
  { value: OFF, label: 'Off' },
];

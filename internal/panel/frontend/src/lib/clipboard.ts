/** What became of a copy attempt. */
export type CopyOutcome = 'copied' | 'unavailable';

/** The one clipboard capability the panel uses, so a test can stand in for it. */
export interface ClipboardWriter {
  writeText(value: string): Promise<void>;
}

/**
 * Put text on the clipboard, saying plainly when the browser would not.
 *
 * Clipboard access is refused outright in an insecure context and, in some
 * browsers, when the gesture that led here is not recognised. A page that
 * claimed success anyway would leave someone pasting whatever they last copied
 * into a pairing prompt, so the caller has to be able to offer the manual route.
 */
export async function copyText(
  value: string,
  writer: ClipboardWriter | undefined,
): Promise<CopyOutcome> {
  if (writer === undefined) {
    return 'unavailable';
  }
  try {
    await writer.writeText(value);
    return 'copied';
  } catch {
    return 'unavailable';
  }
}

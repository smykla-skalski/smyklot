/**
 * Handing a minted link to the app that can claim it.
 *
 * `harness://` is registered by the Harness apps, so on the device being paired
 * the link can be opened directly instead of copied somewhere by hand.
 */

/** The scheme the Harness apps register, as `URL` reports it. */
const PAIRING_SCHEME = 'harness:';

/**
 * The link as something safe to put in an `href`, or `null` when it is not.
 *
 * The value is minted by the daemon rather than typed here, but it still ends up
 * as a navigable URL in the panel's own origin, and `javascript:` or `data:`
 * there would run as the panel. Refusing anything but the one scheme the apps
 * answer costs nothing and keeps that impossible; the raw value stays on screen
 * either way, so a link this rejects can still be copied.
 */
export function pairingHref(value: string): string | null {
  let parsed: URL;
  try {
    parsed = new URL(value);
  } catch {
    return null;
  }
  return parsed.protocol.toLowerCase() === PAIRING_SCHEME ? value : null;
}

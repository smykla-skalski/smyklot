/**
 * Reading a pairing's state the way the page presents it.
 *
 * The daemon owns this vocabulary and the panel passes it through as a string,
 * so everything here treats an unfamiliar state as something to show plainly
 * rather than as an error. A state added to the daemon should reach the page as
 * itself, not as a blank row or a crash.
 */

import type { ChipTone } from '../components/Chip.svelte';
import type { PanelPairing } from './types';

/**
 * States that can no longer become anything else.
 *
 * A revoked pairing is cut off and an expired one lapsed before anyone claimed
 * it, so neither has an unpair worth offering. Anything else, including a state
 * this build has not heard of, keeps the control: refusing to offer it would
 * strand whatever it turns out to be.
 */
const FINISHED = new Set(['expired', 'revoked']);

/**
 * Tone by what the state means, not by which state it is.
 *
 * Brass marks a live credential nobody has spent yet, matching the link card
 * that produced it. Green is a pairing doing its job, red one that was cut off,
 * and grey one that is over.
 */
export function pairingTone(state: string): ChipTone {
  switch (state) {
    case 'pending':
      return 'signal';
    case 'claimed':
    case 'active':
      return 'clear';
    case 'revoked':
      return 'stop';
    case 'expired':
      return 'neutral';
    default:
      return 'neutral';
  }
}

/**
 * Whether the state is something happening now rather than a fixed attribute,
 * which is what the chip's dot means.
 *
 * A claimed pairing whose device has never connected deliberately has no dot:
 * telling it from an active one is the point of having both.
 */
export function pairingIsLive(state: string): boolean {
  return state === 'pending' || state === 'active';
}

/** Whether this pairing still has an unpair worth offering. */
export function pairingCanUnpair(state: string): boolean {
  return !FINISHED.has(state);
}

/** The last thing that happened to a pairing, and when. */
export interface PairingChange {
  label: string;
  at: string;
}

/**
 * When the pairing last changed, and what the change was.
 *
 * Always a moment in the past, so it renders as an age. Each state falls back
 * to when the link was created, because a row that cannot say what happened to
 * it can still say when it started, and a blank column would read as a bug
 * rather than as a missing timestamp.
 */
export function pairingChange(pairing: PanelPairing): PairingChange {
  const created = { label: 'created', at: pairing.created_at };
  switch (pairing.state) {
    case 'revoked':
      // From whichever end carries it: a link withdrawn before any claim has no
      // device to read it from.
      return at('revoked', pairing.revoked_at ?? pairing.device?.revoked_at, created);
    case 'expired':
      return at('expired', pairing.expires_at, created);
    // When it was paired, not when its device was last awake. The account rows
    // already carry that, and saying it twice invites the two to disagree.
    case 'active':
    case 'claimed':
      return at('claimed', pairing.claimed_at, created);
    default:
      return created;
  }
}

function at(label: string, value: string | undefined, fallback: PairingChange): PairingChange {
  return value === undefined ? fallback : { label, at: value };
}

/**
 * What the row is about: the device, or what happened instead of one.
 *
 * A pairing with no device is not a blank row, it is a link at some point in
 * its life, and saying which is what stops "no device" reading as a fault.
 */
export function pairingSubject(pairing: PanelPairing): string {
  if (pairing.device !== undefined) {
    return pairing.device.display_name;
  }
  switch (pairing.state) {
    case 'pending':
      return 'Waiting for a device';
    case 'expired':
      return 'Never claimed';
    case 'revoked':
      return 'Withdrawn before it was claimed';
    default:
      return 'Unclaimed link';
  }
}

/** How much of a minted link is left, as the gauge reports it. */
export type LinkPhase = 'ample' | 'low' | 'warn' | 'critical' | 'spent';

/** The last seconds, where the gauge stops shading and starts flashing. */
export const CRITICAL_MS = 10_000;
/**
 * Shares of the link's own lifetime the gauge changes colour at. Fractions
 * rather than durations, because the lifetime is an operator flag and a fixed
 * threshold would be most of a short link and none of a long one.
 */
const LOW_FRACTION = 0.5;
const WARN_FRACTION = 0.15;

/**
 * Which band a link is in, from what is left of it.
 *
 * Red arrives well before the flash: at the ten-minute default it lands with a
 * minute and a half to go, which is time to do something about it. The flash is
 * the last ten seconds, by which point there is nothing to do but mint another.
 *
 * `leftMs` is `null` when the deadline could not be read; that is not a link
 * running out, it is one whose remaining time is unknown, so it reads as spent
 * rather than as ample.
 */
export function linkPhase(leftMs: number | null, fractionLeft: number): LinkPhase {
  if (leftMs === null || leftMs <= 0) {
    return 'spent';
  }
  if (leftMs <= CRITICAL_MS) {
    return 'critical';
  }
  if (fractionLeft <= WARN_FRACTION) {
    return 'warn';
  }
  return fractionLeft <= LOW_FRACTION ? 'low' : 'ample';
}

/** How many of these are doing something right now, for the plate's header. */
export function liveCount(pairings: PanelPairing[]): number {
  return pairings.filter((pairing) => pairingIsLive(pairing.state)).length;
}

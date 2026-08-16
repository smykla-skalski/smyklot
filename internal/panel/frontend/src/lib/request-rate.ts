/**
 * The same request, over and over, is a defect rather than a busy page.
 *
 * A reactive loop does not look like a loop from the outside. The page renders,
 * nothing flashes, and the only sign is a request log going past faster than it
 * can be read - which nobody is watching in the browser that has it. Two of them
 * shipped: the notification inbox asked from every page of the panel, and the
 * Root settings page asked whenever it was open, both at about 1600 requests a
 * second.
 *
 * `tests/effect-cycles` keeps the shape that caused those out of the source. This
 * is the other half: whatever the cause, an identical address asked for this many
 * times in this long is not something the panel ever does on purpose. Nothing
 * here retries, nothing polls over HTTP - the live updates come down a socket -
 * and a burst of real work varies its query, so a repeat of the exact same
 * address is the signature of a loop and of nothing else.
 */

/** What was asked for too often, and how often. */
export interface RequestFlood {
  address: string;
  count: number;
  withinMs: number;
}

export interface RequestRate {
  /** The flood this request is part of, or null while the rate is ordinary. */
  record(address: string): RequestFlood | null;
}

/**
 * 25 in two seconds.
 *
 * Well above anything the panel does deliberately - the busiest legitimate burst
 * is one request per keystroke, and each of those carries a different query - and
 * far below a loop, which reaches this inside the first tenth of a second.
 */
const LIMIT = 25;
const WINDOW_MS = 2000;

export function createRequestRate(now: () => number = () => Date.now()): RequestRate {
  const seen = new Map<string, number[]>();

  return {
    record(address: string): RequestFlood | null {
      const at = now();
      const times = (seen.get(address) ?? []).filter((time) => at - time < WINDOW_MS);
      times.push(at);
      seen.set(address, times);
      /* Only the addresses being asked for right now are worth keeping. Without
         this the map is a list of every address the session has ever used. */
      if (seen.size > 1) prune(seen, at, WINDOW_MS);

      return times.length > LIMIT ? { address, count: times.length, withinMs: WINDOW_MS } : null;
    },
  };
}

export function floodMessage(flood: RequestFlood): string {
  return (
    `${flood.address} was requested ${flood.count} times in ${flood.withinMs}ms. ` +
    'Something is asking in a loop - look for an effect that depends on state the ' +
    'work it starts writes, and wrap the call in untrack'
  );
}

function prune(seen: Map<string, number[]>, at: number, windowMs: number): void {
  for (const [address, times] of seen) {
    if (times.every((time) => at - time >= windowMs)) seen.delete(address);
  }
}

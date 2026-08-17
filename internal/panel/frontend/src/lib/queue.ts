import { formatCountdown, formatUntil, remainingMs } from './format';
import type { PendingCIRequest, PendingCITrigger } from './types';

/**
 * What the queue says about a request, derived from the reconciler's own rules.
 *
 * All of this was invisible before. The rules live in `internal/pendingci/policy.go`:
 * checks are re-read every 5 minutes while a request is active, a request with nothing new for an
 * hour is deferred to every 6 hours, a repository with no checks at all gets a 10-minute grace
 * before the merge goes ahead, and - the one that matters most to a reader - a passing request
 * waits 30 seconds of quiet before it lands.
 *
 * That last rule is why a row can say "Merging in 0:24" without a new API field: with
 * `last_observed_state = passing` and `next_check_trigger = quiet_period`, `next_check_at` IS the
 * moment the merge happens.
 */

/** How a state is drawn: a tone, a distinct shape, and a word. Never the tone alone. */
export interface QueueState {
  tone: 'clear' | 'stop' | 'neutral' | 'warning' | 'absent';
  icon: 'success' | 'failure' | 'pending' | 'alert' | 'circle-dashed' | 'minus-circle';
  label: string;
}

/**
 * Colour cannot carry this on its own.
 *
 * Measured with the repo's own Viénot simulation: in the dark palette passing and failing separate
 * by 6.20 ΔE00 under protanopia, failing and unreadable by 1.97 under tritanopia, passing and
 * running by 5.25 under deuteranopia. Three of five pairs collapse, so every state here is a tone
 * PLUS a distinctly shaped glyph PLUS a word, and no two share a shape.
 */
export function queueState(request: PendingCIRequest): QueueState {
  switch (request.last_observed_state) {
    case 'passing':
      return { tone: 'clear', icon: 'success', label: 'Passing' };
    case 'failing':
      return { tone: 'stop', icon: 'failure', label: 'Failing' };
    case 'pending':
      return { tone: 'neutral', icon: 'pending', label: 'Running' };
    case 'indeterminate':
      return { tone: 'warning', icon: 'alert', label: 'Unreadable' };
    /* A circle with a bar through it: nothing to wait for, said as a closed
       shape. It used to be the dashed circle, which is the shape below - two
       states drawn identically in one column, which is the exact thing the note
       at the top of this function forbids. The mock names this glyph here too. */
    case 'no_checks':
      return { tone: 'absent', icon: 'minus-circle', label: 'No checks' };
    /* A request between the command and its first reconciliation. Not a fallback
       for a value nobody expects: `last_observed_state` is `NOT NULL DEFAULT ''`
       and `sqlstore.Arm` does not set it, so every request wears this until the
       reconciler first looks. One word, because it is the widest label the column
       carries and the column is sized for its widest label - "Awaiting first
       check" cost 2.75rem in every row of the table, for a state that lasts
       minutes. "Scheduled" is a check that is coming rather than one that is
       missing, which is what "No checks" beside it already means.

       Drawn as an outline not yet filled in, and toned like Running rather than
       like No checks - a look is coming, so it belongs with the state that has
       one in flight. Every pair in this column now differs in at least two of the
       three the note above requires: this one shares a tone with Running and
       nothing else, and no two share a glyph. */
    default:
      return { tone: 'neutral', icon: 'circle-dashed', label: 'Scheduled' };
  }
}

/** The two lines of the "what happens next" column. */
export interface QueueNext {
  lead: string;
  sub: string;
  /** Whether this is a merge landing rather than another look, which the row escalates. */
  merging: boolean;
  /** Seconds left when there is a countdown to run, so the row can escalate near zero. */
  seconds: number | null;
}

export function queueNext(request: PendingCIRequest, nowMs: number): QueueNext {
  /* A request that has finished has no next look, and a countdown on one is a lie about something
     that has already happened. It can still be on the waiting table: the queue holds its
     arrangement while somebody is reading it, so a request that merges under the pointer keeps its
     place until the pointer leaves. What it says there has to be what it now is. */
  if (request.lifecycle !== 'armed') {
    return {
      lead: 'Moves to Recent',
      sub: 'It finished while you were reading it',
      merging: false,
      seconds: null,
    };
  }

  const left = remainingMs(request.next_check_at, nowMs);
  const seconds = left === null ? null : Math.max(0, Math.round(left / 1000));

  /* A passing request in its quiet period is not waiting for another look - the next event IS the
     merge. Nothing in the product said so, and the row that said "next check in 24s" was describing
     the moment the pull request would be merged. */
  if (request.last_observed_state === 'passing' && request.next_check_trigger === 'quiet_period') {
    /* Zero is its own state, not a countdown that happens to read 0:00. The row clears when the
       merge's delivery lands, and between the quiet period elapsing and that arriving there is a
       gap - which the countdown spent sitting at "Merging in 0:00" under the escalation the last
       five seconds turn on, saying a merge was still coming that had already gone. */
    const landing = seconds === null || seconds === 0;
    return {
      lead: landing ? 'Merging now' : `Merging in ${formatCountdown(seconds * 1000)}`,
      sub: landing ? 'Waiting for it to land' : 'Quiet period, then it lands',
      merging: true,
      seconds,
    };
  }

  return {
    lead: `Checks again ${formatUntil(request.next_check_at, nowMs)}`,
    sub: triggerReason(request),
    merging: false,
    seconds,
  };
}

/** Why the next look is scheduled when it is, in the reader's terms rather than the field's. */
function triggerReason(request: PendingCIRequest): string {
  const trigger: PendingCITrigger = request.next_check_trigger;
  if (trigger === 'webhook') return 'A delivery moved it forward';
  if (trigger === 'manual') return 'Asked for from this panel';
  if (trigger === 'cleanup') return 'Tidying up after the merge';
  if (trigger === 'command') return 'First look since it was armed';
  /* Deferred is staleness, not a cadence: nothing about this request has progressed for over an
     hour, so the service stopped watching it closely. */
  if (request.schedule === 'deferred') return 'Nothing has moved for an hour';
  if (request.last_observed_state === 'no_checks') return 'Waiting for checks to appear';

  return 'The regular safety net';
}

/**
 * How long ago, in the width a column has.
 *
 * "24 minutes ago" does not fit and wraps to two lines, which makes one row taller than the rest
 * and breaks the scan the whole page is for. The full timestamp is on the element's `title`, so
 * nothing is lost - this is the glanceable form, and the exact one is a hover away.
 *
 * Every other value here is a number and a unit, and the first minute was the one exception: "just
 * now" is half again as wide as "59 min" and set the width of two columns on its own. `now` is the
 * same fact in the same grammar. The phrase survives where it belongs, in `sinceLabel`.
 */
export function shortAge(value: string, nowMs: number): string {
  const parsed = Date.parse(value);
  if (Number.isNaN(parsed)) return value;
  const elapsed = Math.max(0, nowMs - parsed);
  const minutes = Math.floor(elapsed / 60_000);
  if (minutes < 1) return 'now';
  if (minutes < 60) return `${minutes} min`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} hr`;
  const days = Math.floor(hours / 24);
  if (days < 7) return `${days} d`;

  return `${Math.floor(days / 7)} wk`;
}

/**
 * The same age as a phrase rather than as a measure.
 *
 * `shortAge` is the column's form: a bare `5 min` under a heading that says what is being measured.
 * A sentence needs the preposition, and the first minute already reads as one - `just now ago` is
 * what appending it blindly produces, on the freshest request there is.
 */
export function sinceLabel(value: string, nowMs: number): string {
  const age = shortAge(value, nowMs);
  // An unparseable value comes back untouched, and takes no preposition either. Asked
  // first, so a value that IS the word below is still answered as the value it is.
  if (age === value) return age;
  // A sentence has the room the column did not, so the first minute reads as a phrase here.
  if (age === 'now') return 'just now';

  return `${age} ago`;
}

/** Soonest first: the row at the top of the queue is the row about to move. */
export function bySoonest(first: PendingCIRequest, second: PendingCIRequest): number {
  return first.next_check_at.localeCompare(second.next_check_at);
}

/**
 * Newest first, for the table of things that have already happened.
 *
 * The waiting list is sorted by what happens soonest, and reading that order onto the past puts the
 * oldest outcome at the top - the exact opposite of what somebody scanning for "what just
 * happened" wants.
 */
export function byMostRecent(first: PendingCIRequest, second: PendingCIRequest): number {
  const at = (request: PendingCIRequest): string => request.finished_at ?? request.updated_at;

  return at(second).localeCompare(at(first));
}

/** Whether the label and the reactions have come off since the request finished. */
export interface CleanupState {
  /** What the column's filter selects on, so the menu and the chip cannot part over a word. */
  value: 'done' | 'pending' | 'failed';
  tone: 'clear' | 'warning' | 'stop';
  icon: 'check' | 'pending' | 'alert';
  label: string;
  /** The whole story, for the tooltip that stands in for the label on a narrow screen. */
  detail: string;
}

/**
 * The label says the outcome and not the subject: the column is headed "Cleanup", so a chip
 * reading "Cleanup failed" says the word twice. The whole sentence is on the tooltip, which is
 * where it is needed - on a narrow screen the label goes and the mark is all that is left.
 *
 * What is being tidied is the LABEL and the REACTIONS, on every lifecycle - the pending-CI label,
 * the service's handoff fence, and the bot's reaction on the comment that armed it
 * (`cleanupArtifactsExclusive` in `cmd/github-action/pending_ci_github.go`). It said "the branch
 * and the reaction", and nothing in this path has ever touched a branch: deleting one is a
 * repository setting the org sync manages, a different subsystem entirely. So the sentence named a
 * side effect the service does not have, and a reader watching a cancelled request sit at "Pending"
 * was told the bot was about to delete their branch.
 *
 * "Pending" on a cancelled request is right, and that is worth stating because it looks wrong: a
 * request that never merged still put a label and two reactions on the pull request when it was
 * armed, and those come off the same way a merged one's do.
 */
export function cleanupState(request: PendingCIRequest): CleanupState {
  if (request.cleanup_error !== undefined && request.cleanup_error !== '') {
    return {
      value: 'failed',
      tone: 'stop',
      icon: 'alert',
      label: 'Failed',
      detail: `The label and the reactions could not be taken off: ${request.cleanup_error}`,
    };
  }
  if (request.cleanup_pending) {
    return {
      value: 'pending',
      tone: 'warning',
      icon: 'pending',
      label: 'Pending',
      detail: 'The label and the reactions are still to come off the pull request',
    };
  }

  return {
    value: 'done',
    tone: 'clear',
    icon: 'check',
    label: 'Done',
    detail: 'The label and the reactions have been taken off',
  };
}

/**
 * Why a request ended, in a sentence.
 *
 * The stored `reason` is the service's own words where it has any; the fallbacks are what each
 * lifecycle means, since a row with an empty cell says nothing about what happened to it.
 */
export function endReason(request: PendingCIRequest): string {
  /* Armed again, which is what a new commit on the same pull request does. It can be read here for
     as long as somebody is holding the Recent table still - and "Still waiting" over a row whose
     Outcome column says Scheduled reads as a stall rather than as a fresh start. Checked before the
     stored reason, because that reason belongs to the run that ENDED and this is the next one. */
  if (request.lifecycle === 'armed') return 'Armed again by a new commit';
  if (request.reason !== '') return request.reason;
  if (request.lifecycle === 'merged') return 'Checks passed and stayed quiet for 30 s';
  if (request.lifecycle === 'cancelled') return 'Cancelled before it could merge';
  if (request.lifecycle === 'superseded') return 'Replaced by a later command';

  return 'Still waiting';
}

/** The one-line summary of how a request ended, for the Recent table. */
export function outcomeState(request: PendingCIRequest): QueueState {
  switch (request.lifecycle) {
    case 'merged':
      return { tone: 'clear', icon: 'success', label: 'Merged' };
    case 'cancelled':
      return { tone: 'neutral', icon: 'circle-dashed', label: 'Cancelled' };
    case 'superseded':
      return { tone: 'warning', icon: 'alert', label: 'Superseded' };
    default:
      return queueState(request);
  }
}

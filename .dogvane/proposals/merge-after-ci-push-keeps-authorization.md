---
apiVersion: contracts.dogvane.io/v1alpha1
kind: Proposal
metadata:
  id: merge-after-ci-push-keeps-authorization
  target: merge-after-ci-2026-10-31-reauthorize-waits
  origin: Bart asked on 2026-11-01 that a push no longer needs a Reauthorize click
---

## Change

The rule "a new head never inherits the old authorization" becomes "after a push, Smyklot carries the authorization to the new commit; a base change, or a person who may no longer give the command, still asks for a Reauthorize click".

The replacement is written in `findings/merge-after-ci-2026-11-01-push-keeps-authorization.md`.

## Reason

Bart said on 2026-11-01: "when there is something being pushed after to this approved branch, the approval from the bot is lost, it has to reapprove it instead of waiting for confirmation for authorization". GitHub already drops the bot's approval on a push in his repositories, and the click only repeats what the command already said.

## Expected effect

absence: the Reauthorize click after an ordinary push no longer appears on his pull requests. Signal: no `requested_action` delivery for `reauthorize` after a push in the service history. Window: the remaining checks of the week.

## Decision

Approved on 2026-11-01 by Bart, with the finding named above.

## Result

Filled in when the window closes.

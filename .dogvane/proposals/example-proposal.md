---
apiVersion: contracts.dogvane.io/v1alpha1
kind: Proposal
metadata:
  id: example-proposal
  target: example-change@1
  origin: the check of example-change@1 on 2026-11-03 left one rule undetermined
---

## Change

The rule "the request finishes quickly" becomes "the request finishes within 30 seconds".

## Reason

The check could not decide the rule, because "quickly" names no limit. The agent chose 60 seconds and the check had no evidence either way.

## Expected effect

reduction: undetermined results for this rule. Signal: the undetermined column in `log.md`. Window: the remaining checks of the week.

Other allowed values: absence (a named result no longer appears), pass (a named condition is satisfied), or "no measurable effect claimed".

## Decision

Approved on 2026-11-03. The new contract is `contracts/example-change@2.md`.

## Result

Filled in when the window closes: what the signal showed, and whether it matched the expected effect.

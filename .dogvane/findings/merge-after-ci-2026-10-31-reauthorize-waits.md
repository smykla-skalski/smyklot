---
kind: Finding
component: merge-after-ci
decision: approved
---

# Merge after CI waits for the next tick after a reauthorization

## Facts

- confirmed interface: the commands merge, squash and rebase accept "after ci", and a Reauthorize action sits on the Smyklot check run ([README, merge after CI checks][doc-readme-checks])
- confirmed rule: a new head never inherits the old authorization ([README, merge after CI checks][doc-readme-checks]; shown by [push after the command](#push-after-the-command))
- confirmed rule: the request merges after two passing observations held for the quiet period ([README, merge after CI checks][doc-readme-checks]; shown by [push after the command](#push-after-the-command))
- approved rule: after a person clicks Reauthorize on a waiting request, the request is reconciled within 5 seconds (Bart, 2026-10-31; shown by [push after the command](#push-after-the-command))
- confirmed data: every reconciliation of a request is recorded as an event with a time ([pending-CI events][code-events])
- confirmed dependency: the pending-CI queue lane runs a fallback check every 300 seconds (the policy row `pending_ci` in `queue_policies`, and the [scheduler][code-scheduler])

## Scenarios

### Push after the command

1. On a pull request, write the command "squash after ci".
2. Push one more commit to the pull request.
3. Wait until all CI checks are green.
4. Wait 10 more seconds. Smyklot must not merge.
5. Click Reauthorize on the Smyklot check.
6. Within 5 seconds, Smyklot must record a reconciliation of the request.
7. Wait for the quiet period and 5 more seconds. Smyklot must merge the pull request.

## Violation

The rule "after a person clicks Reauthorize on a waiting request, the request is reconciled within 5 seconds" does not hold.

A reauthorization updates the request and wakes the scheduler. The scheduler does not read the request. It reads the queue item, and the queue item keeps the time of the next fallback check. So the request waits for that check, up to 300 seconds.

On [PR 450][pr-450], Bart reauthorized at 08:31:19. The queue item did not change. The fallback check ran at 08:32:17, exactly 300 seconds after the previous one, and the bot merged at 08:32:23. It looked correct because the fallback was close.

On [PR 475][pr-475], Bart reauthorized at 03:41:25. The queue item did not change. The next fallback check was due at 03:43:10. Bart merged by hand at 03:41:45, and the request was cancelled as superseded. Bart said on 2026-10-31: "after my reauthorization it basically did nothing and I had to merge it".

The evidence is in the service database, read on 2026-10-31: in `queue_events` no transition follows either reauthorization. In the code, [Reauthorize][code-reauthorize] does not synchronize the queue item, while [Wake][code-wake] does.

[doc-readme-checks]: README.md#merge-after-ci-checks "README, section Merge after CI checks"
[code-events]: internal/storage/sqlstore/pending_ci_events.go "pending-CI events"
[code-scheduler]: internal/pendingci/gate/scheduler.go
[code-reauthorize]: internal/storage/sqlstore/pending_ci.go#L743 "Store.Reauthorize"
[code-wake]: internal/storage/sqlstore/pending_ci.go#L492 "Store.Wake"
[pr-450]: https://github.com/smykla-skalski/smyklot/pull/450
[pr-475]: https://github.com/smykla-skalski/smyklot/pull/475

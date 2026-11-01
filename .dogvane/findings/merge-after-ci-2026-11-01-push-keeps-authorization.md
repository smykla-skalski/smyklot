---
kind: Finding
component: merge-after-ci
decision: approved
---

# Merge after CI asks for a click after every push instead of carrying the authorization to the new commit

## Facts

- confirmed interface: the commands merge, squash and rebase accept "after ci", and a Reauthorize action sits on the Smyklot check run ([README, merge after CI checks][doc-readme-checks])
- confirmed interface: Smyklot approves a pull request as a review by the bot user when the person who gave the command has not approved it ([pending-CI approval][code-approval])
- approved rule: after a push to a pull request with a merge-after-CI request, Smyklot keeps the request and carries the authorization to the new commit, with no click and no new command (Bart, 2026-11-01; shown by [push after the command](#push-after-the-command))
- approved rule: a push by anyone carries the authorization, not only a push by the person who gave the command (Bart, 2026-11-01; shown by [push after the command](#push-after-the-command))
- approved rule: after a push, when the bot's approval is gone, Smyklot approves the pull request again in the name of the person who gave the command (Bart, 2026-11-01; shown by [push after the command](#push-after-the-command))
- approved rule: within 5 seconds of GitHub's notice of a push, Smyklot must show its check on the new commit as waiting for CI, with no Reauthorize action (Bart, 2026-11-01; shown by [push after the command](#push-after-the-command))
- approved rule: a request that already waits for a Reauthorize click when this change arrives keeps waiting for the click (Bart, 2026-11-01; shown by [push after the person lost permission](#push-after-the-person-lost-permission))
- approved rule: when Smyklot cannot carry the authorization, because the person who gave the command may no longer give it, Smyklot publishes the Reauthorize action on the new commit as today and does not merge (Bart, 2026-11-01; shown by [push after the person lost permission](#push-after-the-person-lost-permission))
- approved rule: a change of the base branch is not a push and still asks for a Reauthorize click (Bart, 2026-11-01; shown by [base change after the command](#base-change-after-the-command))
- confirmed rule: the request merges the exact new commit after two passing observations held for the quiet period ([README, merge after CI checks][doc-readme-checks]; shown by [push after the command](#push-after-the-command))
- confirmed rule: a person who clicks Reauthorize on a waiting request gets the request reconciled within 5 seconds ([finding: merge after CI waits for the next tick after a reauthorization][doc-finding-reauthorize-waits]; shown by [push after the person lost permission](#push-after-the-person-lost-permission))
- confirmed rule: a pull request converted to draft cancels the request, push or no push ([pending-CI policy][code-policy])
- confirmed dependency: GitHub drops the bot's approval on a push when the branch rules say so, and the rules of this repository say so ([README, merge after CI checks][doc-readme-checks])

## Scenarios

### Push after the command

1. On a pull request, write the command "squash after ci". Smyklot must approve the pull request and show its check on the commit as waiting for CI.
2. Another account pushes one more commit to the pull request. GitHub drops the bot's approval.
3. Within 5 seconds, Smyklot must approve the pull request again.
4. Within 5 seconds, Smyklot must show its check on the new commit as waiting for CI, with no Reauthorize action.
5. Wait until all CI checks on the new commit are green.
6. Wait for the quiet period and 5 more seconds. Smyklot must merge the pull request at the new commit.

### Push after the person lost permission

1. On a pull request, write the command "squash after ci". Smyklot must show its check on the commit as waiting for CI.
2. Remove the person who gave the command from the code owners.
3. Push one more commit to the pull request. GitHub drops the bot's approval.
4. Within 5 seconds, Smyklot must show its check on the new commit with the Reauthorize action. Smyklot must not approve the pull request.
5. Wait until all CI checks on the new commit are green. Wait for the quiet period and 10 more seconds. Smyklot must not merge.
6. Put the person back in the code owners. The person clicks Reauthorize on the Smyklot check.
7. Within 5 seconds, Smyklot must record a reconciliation of the request. Within the quiet period and 5 more seconds, Smyklot must merge the pull request at the new commit.

### Base change after the command

1. On a pull request, write the command "squash after ci". Smyklot must show its check on the commit as waiting for CI.
2. Change the base branch of the pull request.
3. Within 5 seconds, Smyklot must show its check on the commit with the Reauthorize action.
4. Wait until all CI checks are green. Wait for the quiet period and 10 more seconds. Smyklot must not merge.

## Violation

The rule "after a push to a pull request with a merge-after-CI request, Smyklot keeps the request and carries the authorization to the new commit" does not hold.

After a push, Smyklot publishes an action_required check with a Reauthorize action on the new commit and waits for a click. The [policy][code-policy] decides reauthorize for every head change in check mode, and the [reconciler][code-reconciler] publishes the check and records the waiting state. Nothing approves the new commit or reads the permission of the person who gave the command. The [README][doc-readme-checks] says so: "A head or base change never inherits the old authorization."

Bart said on 2026-11-01: "when there is something being pushed after to this approved branch, the approval from the bot is lost, it has to reapprove it instead of waiting for confirmation for authorization".

[doc-readme-checks]: README.md#merge-after-ci-checks "README, section Merge after CI checks"
[doc-finding-reauthorize-waits]: .dogvane/findings/merge-after-ci-2026-10-31-reauthorize-waits.md "Finding: merge after CI waits for the next tick after a reauthorization"
[code-approval]: internal/bot/pendingciapproval.go "pending-CI approval"
[code-policy]: internal/pendingci/policy.go#L74 "revisionChangeDecision"
[code-reconciler]: internal/pendingci/gate/reconciler.go#L274 "Reconciler.requireReauthorization"
[code-reauthorization-check]: internal/pendingci/gate/reauthorization_check.go "the permission and state checks of a reauthorization"

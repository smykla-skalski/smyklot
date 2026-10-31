# Dogvane dogfood week

One week of real use of Dogvane contracts on smyklot. Every change that Bart delegates to an agent gets a contract, a check and a row in `log.md`. No tool is built. The two Dogvane skills, `dogvane-prepare` and `dogvane-check`, are the only prompts, and the contract shape in `dogvane-prepare` is the only contract shape.

The week starts with the first row in `log.md` and ends seven days later.

## Per delegated change

1. Start a timer.
2. Open the `dogvane-prepare` skill and give it the goal in your own words. It reads the project first and asks one question at a time. Answer, or say that you do not know, and it marks the item as open.
3. Read the draft: open items first, then the assumptions grouped by source. Confirm or change them until you approve.
4. On approval, stop the timer. Save the contract as `contracts/<name>@<version>.md`, exactly as the skill wrote it, and note its SHA-256 and the open items you accepted at the top of the findings file for this change.
5. Let the agent implement the contract on a branch or a PR.
6. Run the check with the `dogvane-check` skill: give it the approved contract and the diff, branch or PR.
7. Save the report as `findings/<name>@<version>-<date>.md`. It lists every condition with one of three results: satisfied, not satisfied or undetermined. Failures and unknowns come first, then satisfied conditions, then what was not checked. Each edit outside the scope is listed with file and line.
8. If the contract had to change at any point, write `proposals/<id>.md` with the fields of `proposals/example-proposal.md`, and save the new contract version beside the old one.
9. Add a row to `log.md`.

If you skip the contract for a change, still add a row, with the reason in the skipped column. A skipped contract is a result, not a failure.

## What to count

- Minutes spent on the contract, from the first question to approval.
- Whether the contract was skipped, and why.
- Edits outside the scope that the check caught.
- Conditions the check left undetermined.

## Stop rule

At the end of the week the counts in `log.md` are reviewed. Nothing else is decided.

An approval is a signature file in `.dogvane/approvals/`, made with a hardware key listed in `.dogvane/approvers`, over the finding and the executable scenario. The decision line in a finding is one word, open or approved; the signature is the proof.

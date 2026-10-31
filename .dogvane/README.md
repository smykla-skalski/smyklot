# Dogvane dogfood week

One week of real use of Dogvane on smyklot. Every change that Bart delegates to an agent starts with a finding, is proven by its scenario and gets a row in `log.md`. No tool is built yet. The two Dogvane skills, `dogvane-prepare` and `dogvane-check`, are the only prompts, and the finding shape in `dogvane-prepare` is the only record shape.

The week starts with the first row in `log.md` and ends seven days later.

## Per delegated change

1. Start a timer.
2. Open the `dogvane-prepare` skill and give it the goal in your own words. It reads the project first and asks one question at a time. Answer, or say that you do not know, and it leaves the fact out or marks it assumed.
3. Read the draft: assumed facts first, grouped by what they rest on. Confirm or change them until you approve. Every rule has a scenario that shows it.
4. On approval, stop the timer. The finding lives in `findings/<component>-<date>-<slug>.md` with the decision line set to `approved`.
5. A different agent, or a person, writes the executable form of the scenario in the project's test harness and commits it. The agent that writes the fix never touches it.
6. Sign the approval with the hardware key: the statement is `dogvane approval v1`, the finding hash with the decision line read as `open`, and the scenario hash. Save the signature as `approvals/<finding>.sig`. The signers are listed in `approvers`.
7. Let the agent implement the fix on a branch. The pull request opens with the finding, then the executable scenario, then the fix.
8. Run the check with the `dogvane-check` skill. It verifies the signature first, then runs the scenario and reports each `must` step as satisfied, not satisfied or undetermined.
9. If an approved fact had to change, write `proposals/<id>.md` with the fields of `proposals/example-proposal.md`. Never edit an approved fact in place.
10. Add a row to `log.md`.

If you skip the finding for a change, still add a row, with the reason in the skipped column. A skipped finding is a result, not a failure.

## What to count

- Minutes spent on the finding, from the first question to approval.
- Whether the finding was skipped, and why.
- Edits outside the footprint that the check caught.
- Steps the check left undetermined.
- Whether a failure came from the prompt, the tooling or the idea.

## Stop rule

At the end of the week the counts in `log.md` are reviewed. Nothing else is decided.

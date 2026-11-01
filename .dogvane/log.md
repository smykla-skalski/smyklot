# Dogvane dogfood log

One row per delegated change. See `README.md` for what each column counts.

| Date | Change (PR or branch) | Contract id | Minutes on contract | Skipped (yes or no, reason) | Out-of-scope edits caught | Undetermined results | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 2026-10-31 | https://github.com/smykla-skalski/smyklot/pull/494 | merge-after-ci-2026-10-31-reauthorize-waits | | no | 0 | 0 | first real finding; investigation with database evidence |
| 2026-11-01 | https://github.com/smykla-skalski/smyklot/pull/495 | merge-after-ci-2026-11-01-push-keeps-authorization | 20 | no | 0 | 2 | first signed approval; 8 turns, 0 forks shown (both readers agreed on 13 questions); diff not opened, merged by the bot 6 min after the command; undetermined: no scenario asserts the merged SHA, and the already-waiting rule rests on a unit test; faults: tooling (Apple ssh-keygen lacks FIDO, a base retarget fires no CI, GraphQL retarget error), prompt (first reading of the goal cost 2 turns) |

# Retro figures

## Runs

| Run | Sessions | Forks | Cost | Verify gate runs | Verify reworks | Gate-fixer sessions | Review rounds |
| --- | --- | --- | --- | --- | --- | --- | --- |
| adopt-redesign | 15 | 36 | $3.2 | 0 | 0 | 1 | none |
| adopt-worker-record | 24 | 30 | $38.96 | 3 | 1 | 8 | code 1, plan 2 |
| codex-same-as-claude | 15 | 35 | $3.53 | 0 | 0 | 0 | none |
| creel-notice-hygiene | 7 | 12 | $9.49 | 4 | 2 | 1 | code 1 |
| decision-record-replace | 12 | 32 | $1.28 | 0 | 0 | 0 | none |
| gate-scope-by-coverage-map | 15 | 31 | $1.83 | 0 | 0 | 0 | none |
| plan-writer-context | 1 | 0 | $2.62 | 1 | 0 | 0 | none |
| test-only-functions-in-prod | 11 | 24 | $31.94 | 10 | 7 | 2 | code 2 |
| testmap-upkeep-design | 10 | 28 | $2.2 | 0 | 0 | 0 | none |
| weaver | 14 | 33 | $2.06 | 0 | 0 | 0 | none |

## Producers against their medians

| Row | Sessions | Median turns | Median cost | Outliers |
| --- | --- | --- | --- | --- |
| Code-Bouncer | 7 | 6 | $0.13 | - |
| Code-Burler-Fix | 4 | 45 | $2.08 | test-only-functions-in-prod 32cc24c5 (62 turns, $4.77) |
| Code-Burler-Review | 4 | 25.5 | $1.63 | test-only-functions-in-prod 6b616569 (36 turns, $5.4) |
| Darn-Write | 3 | 59 | $6.52 | test-only-functions-in-prod 50336444 (99 turns, $17.87) |
| Describe | 3 | 8 | $0.14 | - |
| Friction-Reflect | 1 | 7 | $0.06 | - |
| Parent-Align | 1 | 14 | $0.48 | - |
| Plan-Bouncer | 3 | 6 | $0.13 | - |
| Plan-Burler-Fix | 2 | 21 | $2.26 | - |
| Plan-Burler-Review | 2 | 34 | $3.84 | - |
| Plan-Write | 1 | 41 | $8.13 | - |
| Raddle-File | 1 | 4 | $0 | - |
| Raddle-Write | 6 | 26.5 | $2.13 | - |
| Webster | 1 | 25 | $13.47 | - |

## Serial single lookups

Turns that made one read or search right after a turn that did the same, per session with any.

| Run | Session | Kind | Title | Turns | Lookup turns | Serial |
| --- | --- | --- | --- | --- | --- | --- |
| test-only-functions-in-prod | 50336444 | main | ly:test-only-functions-in-prod:darn | 99 | 39 | 19 |
| creel-notice-hygiene | c2df8481 | main | ly:creel-notice-hygiene:darn | 59 | 27 | 15 |
| test-only-functions-in-prod | e05851d3 | main | ly:test-only-functions-in-prod:code-burler-review | 36 | 21 | 15 |
| adopt-redesign | 367d42b0 | main | ly:adopt-redesign:discussion-burler-review | 30 | 18 | 11 |
| weaver | 376dc64f | main | ly:weaver:discussion-burler-fix | 26 | 14 | 11 |
| codex-same-as-claude | a23c382e | main | ly:codex-same-as-claude:discussion | 49 | 20 | 11 |
| adopt-worker-record | f6b42ead | main | ly:adopt-worker-record:plan | 41 | 18 | 10 |
| test-only-functions-in-prod | 6b616569 | main | ly:test-only-functions-in-prod:code-burler-review | 36 | 14 | 9 |
| weaver | e17f1c54 | main | ly:weaver:arbiter | 17 | 11 | 9 |
| codex-same-as-claude | fb901d5b | main | ly:codex-same-as-claude:discussion-burler-fix | 36 | 18 | 9 |
| test-only-functions-in-prod | agent-ac | fork |  | 32 | 15 | 8 |
| codex-same-as-claude | 9c6f8bf9 | main | ly:codex-same-as-claude:raddle | 40 | 14 | 8 |
| adopt-worker-record | b81b2100 | main | ly:adopt-worker-record:code-burler-fix | 53 | 16 | 7 |
| decision-record-replace | 91782c57 | main | ly:decision-record-replace:discussion-burler-fix | 28 | 12 | 7 |
| decision-record-replace | f7d6690b | main | ly:decision-record-replace:discussion | 32 | 15 | 7 |
| testmap-upkeep-design | f7477c9e | main | ly:testmap-upkeep-design:raddle | 27 | 10 | 7 |
| weaver | agent-ae | fork |  | 11 | 9 | 7 |
| weaver | agent-a4 | fork |  | 11 | 9 | 7 |
| codex-same-as-claude | agent-a9 | fork |  | 9 | 8 | 7 |
| adopt-worker-record | agent-ad | fork |  | 25 | 11 | 6 |
| adopt-worker-record | agent-ae | fork |  | 36 | 11 | 6 |
| adopt-worker-record | agent-a2 | fork |  | 11 | 8 | 6 |
| gate-scope-by-coverage-map | 6dd4e936 | main | ly:gate-scope-by-coverage-map:discussion-burler-review | 25 | 10 | 6 |
| decision-record-replace | 2c77f780 | main | ly:decision-record-replace:discussion-burler-review | 28 | 10 | 6 |
| creel-notice-hygiene | bf4744e3 | main | ly:creel-notice-hygiene:code-burler-fix | 37 | 12 | 6 |
| plan-writer-context | db42edb9 | main | ly:plan-writer-context:darn | 58 | 15 | 6 |
| adopt-redesign | agent-aa | fork |  | 10 | 8 | 6 |
| codex-same-as-claude | bcd212dc | main | ly:codex-same-as-claude:discussion-burler-fix | 21 | 10 | 6 |
| gate-scope-by-coverage-map | ba288737 | main | ly:gate-scope-by-coverage-map:discussion-burler-fix | 21 | 8 | 5 |
| adopt-redesign | agent-ae | fork |  | 9 | 7 | 5 |
| adopt-redesign | agent-aa | fork |  | 8 | 6 | 5 |
| adopt-redesign | dcf91848 | main | ly:adopt-redesign:discussion-burler-review | 21 | 7 | 5 |
| weaver | 5cb15bf8 | main | ly:weaver:arbiter | 19 | 9 | 5 |
| weaver | 63ce7b66 | main | ly:weaver:discussion-burler-fix | 24 | 10 | 5 |
| weaver | db2dad3a | main | ly:weaver:raddle | 27 | 10 | 5 |
| adopt-worker-record | agent-a3 | fork |  | 7 | 5 | 4 |
| gate-scope-by-coverage-map | d41e5571 | main | ly:gate-scope-by-coverage-map:arbiter | 10 | 6 | 4 |
| decision-record-replace | agent-aa | fork |  | 7 | 5 | 4 |
| decision-record-replace | agent-a3 | fork |  | 8 | 6 | 4 |
| testmap-upkeep-design | ef8445a1 | main | ly:testmap-upkeep-design:discussion-burler-review | 27 | 9 | 4 |
| testmap-upkeep-design | agent-af | fork |  | 7 | 5 | 4 |
| adopt-redesign | 7a322b9b | main | ly:adopt-redesign:discussion-burler-review | 20 | 8 | 4 |
| test-only-functions-in-prod | a3c9eacf | main | ly:test-only-functions-in-prod:gate-fixer | 24 | 8 | 4 |
| codex-same-as-claude | 6901d4a2 | main | ly:codex-same-as-claude:discussion-burler-review | 29 | 8 | 4 |
| codex-same-as-claude | agent-ab | fork |  | 6 | 5 | 4 |
| codex-same-as-claude | agent-a8 | fork |  | 7 | 5 | 4 |
| adopt-worker-record | 1e79a59d | main | ly:adopt-worker-record:plan-burler-fix | 20 | 6 | 3 |
| adopt-worker-record | agent-a2 | fork |  | 6 | 4 | 3 |
| adopt-worker-record | agent-ab | fork |  | 6 | 4 | 3 |
| adopt-worker-record | agent-a3 | fork |  | 54 | 12 | 3 |
| adopt-worker-record | dd8a027e | main | ly:adopt-worker-record:plan-burler-review | 40 | 10 | 3 |
| adopt-worker-record | agent-a2 | fork |  | 6 | 4 | 3 |
| adopt-worker-record | agent-a3 | fork |  | 6 | 4 | 3 |
| adopt-worker-record | ebf21251 | main | ly:adopt-worker-record:gate-fixer | 16 | 5 | 3 |
| adopt-worker-record | agent-ab | fork |  | 7 | 4 | 3 |
| gate-scope-by-coverage-map | 2fb78e3e | main | ly:gate-scope-by-coverage-map:discussion | 22 | 5 | 3 |
| gate-scope-by-coverage-map | agent-a2 | fork |  | 7 | 5 | 3 |
| gate-scope-by-coverage-map | agent-a6 | fork |  | 7 | 4 | 3 |
| gate-scope-by-coverage-map | agent-a6 | fork |  | 7 | 5 | 3 |
| gate-scope-by-coverage-map | agent-a1 | fork |  | 6 | 4 | 3 |
| decision-record-replace | 047be39d | main | ly:decision-record-replace:discussion-burler-review | 24 | 7 | 3 |
| decision-record-replace | d64cb603 | main | ly:decision-record-replace:raddle | 22 | 6 | 3 |
| decision-record-replace | agent-a3 | fork |  | 5 | 4 | 3 |
| decision-record-replace | e44fb294 | main | ly:decision-record-replace:discussion-burler-review | 31 | 7 | 3 |
| decision-record-replace | agent-a5 | fork |  | 7 | 5 | 3 |
| testmap-upkeep-design | 6aadbda2 | main | ly:testmap-upkeep-design:discussion-burler-review | 20 | 5 | 3 |
| testmap-upkeep-design | agent-a3 | fork |  | 7 | 4 | 3 |
| testmap-upkeep-design | agent-a5 | fork |  | 6 | 4 | 3 |
| testmap-upkeep-design | agent-ab | fork |  | 7 | 5 | 3 |
| adopt-redesign | agent-a6 | fork |  | 8 | 5 | 3 |
| adopt-redesign | agent-a8 | fork |  | 6 | 4 | 3 |
| adopt-redesign | agent-ab | fork |  | 6 | 4 | 3 |
| test-only-functions-in-prod | 1a8d7feb | main | ly:test-only-functions-in-prod:describe | 10 | 5 | 3 |
| weaver | agent-ab | fork |  | 6 | 4 | 3 |
| weaver | agent-a7 | fork |  | 9 | 6 | 3 |
| weaver | agent-aa | fork |  | 6 | 4 | 3 |
| weaver | agent-aa | fork |  | 6 | 4 | 3 |
| weaver | 64b6597a | main | ly:weaver:discussion-burler-review | 24 | 7 | 3 |
| weaver | agent-a1 | fork |  | 7 | 4 | 3 |
| adopt-worker-record | 14858312 | main | ly:adopt-worker-record:gate-fixer | 11 | 4 | 2 |
| adopt-worker-record | 3fb5062c | main | ly:adopt-worker-record:plan-burler-review | 28 | 4 | 2 |
| adopt-worker-record | agent-ab | fork |  | 7 | 4 | 2 |
| gate-scope-by-coverage-map | 177800b4 | main | ly:gate-scope-by-coverage-map:discussion-burler-review | 20 | 5 | 2 |
| gate-scope-by-coverage-map | agent-a6 | fork |  | 6 | 3 | 2 |
| gate-scope-by-coverage-map | agent-af | fork |  | 5 | 3 | 2 |
| gate-scope-by-coverage-map | 43251e2e | main | ly:gate-scope-by-coverage-map:arbiter | 9 | 4 | 2 |
| gate-scope-by-coverage-map | 6b70c912 | main | ly:gate-scope-by-coverage-map:discussion-burler-review | 24 | 6 | 2 |
| gate-scope-by-coverage-map | acf8e486 | main | ly:gate-scope-by-coverage-map:raddle | 23 | 4 | 2 |
| gate-scope-by-coverage-map | agent-a7 | fork |  | 5 | 3 | 2 |
| decision-record-replace | agent-a2 | fork |  | 5 | 3 | 2 |
| decision-record-replace | agent-a8 | fork |  | 5 | 3 | 2 |
| decision-record-replace | agent-a5 | fork |  | 5 | 3 | 2 |
| decision-record-replace | agent-ae | fork |  | 6 | 3 | 2 |
| decision-record-replace | agent-ad | fork |  | 6 | 3 | 2 |
| decision-record-replace | agent-af | fork |  | 6 | 4 | 2 |
| testmap-upkeep-design | f701b17a | main | ly:testmap-upkeep-design:discussion | 28 | 9 | 2 |
| testmap-upkeep-design | agent-a8 | fork |  | 5 | 3 | 2 |
| testmap-upkeep-design | agent-ab | fork |  | 5 | 3 | 2 |
| testmap-upkeep-design | agent-a2 | fork |  | 4 | 3 | 2 |
| testmap-upkeep-design | agent-ac | fork |  | 5 | 3 | 2 |
| testmap-upkeep-design | agent-af | fork |  | 4 | 3 | 2 |
| creel-notice-hygiene | 98b61628 | main | ly:creel-notice-hygiene:code-burler-review | 15 | 5 | 2 |
| creel-notice-hygiene | agent-a3 | fork |  | 10 | 3 | 2 |
| adopt-redesign | 26dcec0c | main | ly:adopt-redesign:discussion | 28 | 9 | 2 |
| adopt-redesign | agent-ad | fork |  | 6 | 3 | 2 |
| adopt-redesign | 43042c08 | main | ly:adopt-redesign:discussion-burler-fix | 17 | 5 | 2 |
| adopt-redesign | 5c53aec1 | main | ly:adopt-redesign:discussion-burler-fix | 19 | 6 | 2 |
| adopt-redesign | agent-a3 | fork |  | 7 | 4 | 2 |
| adopt-redesign | bcc58c73 | main | ly:adopt-redesign:discussion-burler-fix | 17 | 5 | 2 |
| test-only-functions-in-prod | 32cc24c5 | main | ly:test-only-functions-in-prod:code-burler-fix | 62 | 12 | 2 |
| test-only-functions-in-prod | agent-a7 | fork |  | 29 | 7 | 2 |
| test-only-functions-in-prod | agent-ad | fork |  | 17 | 4 | 2 |
| weaver | agent-a0 | fork |  | 8 | 4 | 2 |
| weaver | 537d8b39 | main | ly:weaver:discussion-burler-review | 19 | 6 | 2 |
| weaver | agent-a7 | fork |  | 5 | 3 | 2 |
| weaver | agent-a7 | fork |  | 5 | 3 | 2 |
| weaver | agent-aa | fork |  | 5 | 3 | 2 |
| weaver | agent-a2 | fork |  | 5 | 3 | 2 |
| weaver | ebad9215 | main | ly:weaver:discussion-burler-review | 17 | 5 | 2 |
| codex-same-as-claude | 0ab11620 | main | ly:codex-same-as-claude:discussion-burler-review | 20 | 6 | 2 |
| codex-same-as-claude | 9a2cc575 | main | ly:codex-same-as-claude:discussion-burler-review | 23 | 7 | 2 |
| codex-same-as-claude | agent-a6 | fork |  | 6 | 3 | 2 |
| codex-same-as-claude | agent-a6 | fork |  | 6 | 4 | 2 |
| codex-same-as-claude | agent-ac | fork |  | 8 | 5 | 2 |
| codex-same-as-claude | b05e985c | main | ly:codex-same-as-claude:arbiter | 9 | 3 | 2 |
| codex-same-as-claude | d503408f | main | ly:codex-same-as-claude:arbiter | 10 | 5 | 2 |
| codex-same-as-claude | agent-a2 | fork |  | 6 | 3 | 2 |
| adopt-worker-record | 06a8f9aa | main | ly:adopt-worker-record:fixer | 14 | 5 | 1 |
| adopt-worker-record | 67d44b9c | main | ly:adopt-worker-record:code-burler-review | 15 | 4 | 1 |
| adopt-worker-record | c47fc0aa | main | ly:adopt-worker-record:plan-bouncer-seed | 7 | 2 | 1 |
| adopt-worker-record | agent-a8 | fork |  | 4 | 2 | 1 |
| adopt-worker-record | agent-ac | fork |  | 13 | 5 | 1 |
| adopt-worker-record | f8adad27 | main | ly:adopt-worker-record:plan-burler-fix | 22 | 4 | 1 |
| adopt-worker-record | agent-a3 | fork |  | 4 | 2 | 1 |
| gate-scope-by-coverage-map | agent-a3 | fork |  | 4 | 2 | 1 |
| gate-scope-by-coverage-map | agent-a6 | fork |  | 4 | 2 | 1 |
| gate-scope-by-coverage-map | agent-aa | fork |  | 4 | 2 | 1 |
| gate-scope-by-coverage-map | agent-aa | fork |  | 5 | 2 | 1 |
| gate-scope-by-coverage-map | agent-ac | fork |  | 5 | 2 | 1 |
| gate-scope-by-coverage-map | agent-a9 | fork |  | 4 | 2 | 1 |
| decision-record-replace | agent-a5 | fork |  | 4 | 2 | 1 |
| decision-record-replace | agent-ae | fork |  | 5 | 2 | 1 |
| decision-record-replace | 83c4d476 | main | ly:decision-record-replace:discussion-burler-fix | 16 | 5 | 1 |
| decision-record-replace | agent-a8 | fork |  | 4 | 2 | 1 |
| decision-record-replace | agent-a0 | fork |  | 4 | 2 | 1 |
| decision-record-replace | e970e98c | main | ly:decision-record-replace:discussion-burler-fix | 21 | 6 | 1 |
| decision-record-replace | agent-a2 | fork |  | 5 | 2 | 1 |
| decision-record-replace | agent-a3 | fork |  | 6 | 2 | 1 |
| testmap-upkeep-design | 944fa3f3 | main | ly:testmap-upkeep-design:discussion-burler-fix | 14 | 3 | 1 |
| testmap-upkeep-design | agent-a0 | fork |  | 4 | 2 | 1 |
| testmap-upkeep-design | agent-a4 | fork |  | 4 | 2 | 1 |
| testmap-upkeep-design | agent-a1 | fork |  | 4 | 2 | 1 |
| testmap-upkeep-design | f206ee83 | main | ly:testmap-upkeep-design:discussion-burler-fix | 24 | 8 | 1 |
| creel-notice-hygiene | 5b07e0cb | main | ly:creel-notice-hygiene:describe | 8 | 3 | 1 |
| creel-notice-hygiene | agent-a3 | fork |  | 9 | 3 | 1 |
| creel-notice-hygiene | agent-a6 | fork |  | 8 | 2 | 1 |
| adopt-redesign | agent-af | fork |  | 4 | 2 | 1 |
| adopt-redesign | agent-a4 | fork |  | 4 | 2 | 1 |
| adopt-redesign | 4cbbd15e | main | ly:adopt-redesign:raddle | 26 | 5 | 1 |
| adopt-redesign | agent-a2 | fork |  | 4 | 2 | 1 |
| adopt-redesign | agent-af | fork |  | 4 | 2 | 1 |
| adopt-redesign | agent-a9 | fork |  | 4 | 2 | 1 |
| test-only-functions-in-prod | agent-a4 | fork |  | 13 | 2 | 1 |
| test-only-functions-in-prod | agent-a4 | fork |  | 15 | 5 | 1 |
| test-only-functions-in-prod | agent-a6 | fork |  | 20 | 4 | 1 |
| test-only-functions-in-prod | agent-a8 | fork |  | 12 | 2 | 1 |
| test-only-functions-in-prod | agent-ab | fork |  | 10 | 3 | 1 |
| test-only-functions-in-prod | agent-ae | fork |  | 13 | 5 | 1 |
| test-only-functions-in-prod | ffc1ed10 | main | ly:test-only-functions-in-prod:code-burler-fix | 32 | 6 | 1 |
| test-only-functions-in-prod | agent-a0 | fork |  | 4 | 2 | 1 |
| weaver | 2232920d | main | ly:weaver:discussion-burler-fix | 19 | 4 | 1 |
| weaver | agent-a1 | fork |  | 4 | 2 | 1 |
| weaver | agent-ae | fork |  | 4 | 2 | 1 |
| weaver | agent-a1 | fork |  | 4 | 2 | 1 |
| weaver | 525590c8 | main | ly:weaver:discussion | 25 | 6 | 1 |
| weaver | agent-a9 | fork |  | 4 | 2 | 1 |
| weaver | agent-ae | fork |  | 5 | 2 | 1 |
| weaver | agent-a8 | fork |  | 6 | 3 | 1 |
| weaver | agent-a8 | fork |  | 4 | 2 | 1 |
| weaver | agent-a1 | fork |  | 5 | 3 | 1 |
| codex-same-as-claude | 186a85fc | main | ly:codex-same-as-claude:arbiter | 14 | 4 | 1 |
| codex-same-as-claude | agent-a1 | fork |  | 4 | 2 | 1 |
| codex-same-as-claude | agent-a9 | fork |  | 4 | 2 | 1 |
| codex-same-as-claude | agent-ab | fork |  | 5 | 2 | 1 |
| codex-same-as-claude | agent-ac | fork |  | 4 | 2 | 1 |
| codex-same-as-claude | agent-a0 | fork |  | 5 | 3 | 1 |
| codex-same-as-claude | agent-a5 | fork |  | 7 | 3 | 1 |
| codex-same-as-claude | agent-a8 | fork |  | 5 | 2 | 1 |
| codex-same-as-claude | agent-af | fork |  | 4 | 2 | 1 |
| codex-same-as-claude | agent-af | fork |  | 7 | 3 | 1 |
| codex-same-as-claude | agent-a8 | fork |  | 4 | 2 | 1 |
| codex-same-as-claude | agent-ab | fork |  | 4 | 2 | 1 |

## Refused commands

| Refusal | Count |
| --- | --- |
| sed -i, perl -i, and a heredoc, redirect or tee into a sourc | 3 |
| python and python3 are refused in every lym session, a versi | 1 |
| rm on a computed path, one built from a variable or a comman | 1 |

| Run | Session | Kind | Turn | Tool | Refusal |
| --- | --- | --- | --- | --- | --- |
| decision-record-replace | 2c77f780 | main | 21 | Bash | rm on a computed path, one built from a variable or a command substitution, is refused here because  |
| decision-record-replace | 91782c57 | main | 20 | Bash | sed -i, perl -i, and a heredoc, redirect or tee into a source file are refused here; change a file b |
| decision-record-replace | e44fb294 | main | 14 | Bash | sed -i, perl -i, and a heredoc, redirect or tee into a source file are refused here; change a file b |
| test-only-functions-in-prod | agent-ae | fork | 7 | Bash | sed -i, perl -i, and a heredoc, redirect or tee into a source file are refused here; change a file b |
| codex-same-as-claude | agent-ac | fork | 3 | Bash | python and python3 are refused in every lym session, a version check included, so never probe for Py |

## Agent gate runs

| Run | Session | Title | Gate runs | Failed |
| --- | --- | --- | --- | --- |
| adopt-worker-record | 06a8f9aa | ly:adopt-worker-record:fixer | 2 | 0 |
| adopt-worker-record | 2224813f | ly:adopt-worker-record:gate-fixer | 1 | 0 |
| adopt-worker-record | 3b927e1b | ly:adopt-worker-record:gate-fixer | 1 | 0 |
| adopt-worker-record | agent-a3 |  | 8 | 0 |
| adopt-worker-record | agent-a4 |  | 3 | 0 |
| adopt-worker-record | agent-ad |  | 4 | 0 |
| adopt-worker-record | agent-ae |  | 2 | 0 |
| adopt-worker-record | 8c4f8dd4 | ly:adopt-worker-record:gate-fixer | 1 | 0 |
| adopt-worker-record | b81b2100 | ly:adopt-worker-record:code-burler-fix | 5 | 0 |
| adopt-worker-record | bdfb5ae6 | ly:adopt-worker-record:gate-fixer | 2 | 1 |
| adopt-worker-record | c43a3273 | ly:adopt-worker-record:gate-fixer | 6 | 0 |
| adopt-worker-record | ebf21251 | ly:adopt-worker-record:gate-fixer | 2 | 0 |
| adopt-worker-record | fb4917be | ly:adopt-worker-record:gate-fixer | 2 | 1 |
| gate-scope-by-coverage-map | agent-a2 |  | 1 | 0 |
| creel-notice-hygiene | bf4744e3 | ly:creel-notice-hygiene:code-burler-fix | 5 | 0 |
| creel-notice-hygiene | ec5b4984 | ly:creel-notice-hygiene:gate-fixer | 1 | 0 |
| test-only-functions-in-prod | 32cc24c5 | ly:test-only-functions-in-prod:code-burler-fix | 6 | 1 |
| test-only-functions-in-prod | 4fecb479 | ly:test-only-functions-in-prod:gate-fixer | 1 | 0 |
| test-only-functions-in-prod | a3c9eacf | ly:test-only-functions-in-prod:gate-fixer | 2 | 1 |
| test-only-functions-in-prod | ffc1ed10 | ly:test-only-functions-in-prod:code-burler-fix | 4 | 0 |

## Review findings by column

Each review perch's rounds summed over the runs; a column is a severity or a finding kind of the facts table.

| Perch | Column | Total |
| --- | --- | --- |
| code | BLOCKING | 3 |
| code | Findings | 40 |
| code | Gating | 11 |
| code | LOW | 20 |
| code | MEDIUM | 12 |
| code | NIT | 5 |
| code | consistency | 19 |
| code | decision | 1 |
| code | design | 20 |
| code | scope | 0 |
| plan | BLOCKING | 1 |
| plan | Findings | 19 |
| plan | Gating | 7 |
| plan | LOW | 8 |
| plan | MEDIUM | 9 |
| plan | NIT | 1 |
| plan | consistency | 9 |
| plan | decision | 1 |
| plan | design | 8 |
| plan | scope | 1 |

## Ledger keys open in more than one round


## Missing records


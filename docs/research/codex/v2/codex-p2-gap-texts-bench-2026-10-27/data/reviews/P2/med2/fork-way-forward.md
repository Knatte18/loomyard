| Refusal | Planned text and next step | First failing precondition |
|---|---|---|
| `plan-language-mismatch` | Set `language:` to the files’ language, or `none` for mixed code. | None for the plan-time cases prescribed. |
| C# `glyph-malformed` under Go | Set `language: csharp`. | None for the named C# member input. |
| `rename-namespace-pair` | Replace namespace Rename with Edit cards covering declarations and references, or correct the old side to an existing root file. | None. |
| Cross-alphabet ambiguity | Names both languages; name the file or a member. | None attributable to the plan; this implements D6’s prescribed recovery. |
| C# `rename-old-unresolved`, malformed new handle | Spell the new handle in the C# alphabet. | None. |
| C# `rename-old-unresolved`, unsupported declared token | Drop the pair; rename with the type where applicable. | None in the recovery instruction itself. |
| C# `resign-head-mismatch` | Preserve parameter-list presence, or express a kind change as Delete/Create. | None. |
| `resign-overload-collision` | Use a free signature, remove/re-sign the existing overload earlier, or drop a conflicting declaration. | None; the alternatives correspond to D7’s exceptions. |
| `resign-not-done` | Make the edit match the head and recover; alternatively correct the head, `rebaseline --card NN`, then recover the batch. | None for a newly failed batch. The existing recovery budget remains applicable. |

No findings from the way-forward lens.

The post-batch failure route preserves the proposed recovery: `failCardNotDone` calls `failBatch`, which archives the OK report and marks the batch terminal failed (`internal/websterengine/recordbatch.go:134`, `internal/websterengine/batchfail.go:91`).
That state is editable through `Rebaseline` and exempt from the “already has an OK report” recovery refusal.
The CLI already tests editing a failed card, rebaselining it, and recovering it (`internal/webstercli/verbs_test.go:942`).
The new automatic rewrite’s `restampFingerprint` also advances begun-card hashes, not merely the aggregate fingerprint (`internal/websterengine/fingerprint.go:205`).

Tool-call manifest:
1. Read rebaseline and recovery definitions and references; the inherited manifest did not contain the commands named by card 12’s way forward.
2. Read post-batch failure propagation, recovery guards, and the rebaseline CLI; needed to establish the state left by the refusal.
3. Read root/webster pre-runs, recovery CLI entry, and fingerprint references; needed to check upstream wiring and persistence.
4. Read denied-recovery rules, Create ambiguity rendering, and fingerprint implementation; needed to check legal instructions and hash advancement.
5. Located `batchfail.go`, read recovery dependencies and the existing failed-card rebaseline/recovery test; needed the missing failure primitive and execution evidence.
6. Read `failBatch` and hub/standalone wiring; needed to confirm terminal state, report archival, and config preconditions.

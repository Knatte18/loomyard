40 chars + newline, correct SHA-1: `8685f63b92674c989d8e58771cf28f9f1a6547d6`.

Summary: 3 of 3 cards committed this session (cards 12, 13, 14). The measurement record in `internal/reedengine/doc.go` recorded "no candidate accepted" (Branch C), so all three cards followed the no-functional-change branch: card 12 added a documentation-only paragraph to `internal/reedengine/windowsize.go`'s `installResizePinsLocked` doc comment; card 13 added `TestResizePinHookArgvs_NoRepaintEntryShips` to `internal/reedengine/windowsize_test.go`; card 14 added the inverted `TestSmokeDotFillResizeTreatment` tripwire to `internal/reedcli/smoke_dotfill_test.go`, with a follow-up fix commit correcting the poll timing (sampling before the grow clears the artifact, mirroring the existing control's proven sequencing). Both verify sub-invocations pass. Working tree is clean.

Relevant files:
- `/home/knatte/Code/loomyard/wts/reed-attach-dotfill-artifact/internal/reedengine/windowsize.go`
- `/home/knatte/Code/loomyard/wts/reed-attach-dotfill-artifact/internal/reedengine/windowsize_test.go`
- `/home/knatte/Code/loomyard/wts/reed-attach-dotfill-artifact/internal/reedcli/smoke_dotfill_test.go`

{"status":"success","commit_sha":"8685f63b92674c989d8e58771cf28f9f1a6547d6","session_id":"edf51a56-89df-41df-92b8-1c43cfa209d1","cards_done":[12,13,14]}

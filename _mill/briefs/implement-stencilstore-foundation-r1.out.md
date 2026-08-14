All 6 of 6 cards committed successfully; verify passed cleanly on the first run with no failures or self-fixes needed.

Summary of work in `/home/knatte/Code/loomyard/wts/stencils-directory-reorg`:
- `internal/stencil/stencil.go` — exported `StripLeadingComment` and `TopLevelMarkers`, refactored `unfilledTopLevelMarkers` to share the new `topLevelMarkerNames` AST-walking helper.
- `internal/stencil/export_test.go` — new tests for the two exports plus a `Fill` regression pin.
- `internal/stencilstore/doc.go`, `internal/stencilstore/stencilstore.go` — new package: `NormalizeLF`, `BodyHash`, `ParseStamp`, `ApplyStamp`, `Mode`, `Registry`, `RelPath`, `Path`, `State`/`Classify`.
- `internal/stencilstore/reconcile.go` — `Read`, `Reconcile`, `ForceRefresh`, `.gitattributes` seeding, port-back drift warning.
- `internal/stencilstore/validate.go` — `Finding`, `Validate`.
- `internal/stencilstore/stencilstore_test.go`, `reconcile_test.go`, `validate_test.go` — full hermetic `t.TempDir()` coverage with a `fakeRegistry`.
- `internal/fabricengine/junctionnames.go` — added `stencilsDirName` and `StencilsDir(hub)`.
- `internal/fabricengine/stencilsdir_test.go` — new tests for `StencilsDir`.

{"status":"success","commit_sha":"ca1e65acc860f90163350b2f279c2899345ce02f","session_id":"2a017dfc-d07f-4960-b0dd-ad31caf3b990","cards_done":[1,2,3,4,5,6]}

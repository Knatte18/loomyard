All 3 declared cards committed (3 of 3), matching the batch's `cards: 3`. Working tree is clean. Verify (`go test ./internal/lyxcwd/...`) passed.

Card-count self-check: 3 of 3 cards committed — all complete.

Summary of work:
- `/home/knatte/Code/loomyard/wts/standalone-docs-and-invariants/CONSTRAINTS.md` — added the new `## Told-Geometry Invariant` section (three resolution tiers, producer/orchestrator split with `ResolveMode`'s degrade/refuse outcomes, hubgeom/standalonegeom adapter direction, mode trigger, and the enforcement-basis/membership predicate with the six machine-enforced tests and eight review-obligation packages), and added a new bullet to `## Cwd Resolution Invariant` stating the four things `lyxcwd.Resolve` validates and what it does not prove. Every factual claim was verified against `internal/preflight/predicates.go`, `internal/preflight/doc.go`, `internal/hubgeom`, `internal/standalonegeom`, `internal/lyxcwd/lyxcwd.go`, `internal/lyxcwd/anchor.go`, and the six named leaf/seam enforcement test files before writing.

{"status":"success","commit_sha":"4a2cc5689a1c4e9f095e52a17029c88542c75bcb","session_id":"64e89011-89ef-4733-8914-ff94763e2121","cards_done":[1,2,3]}

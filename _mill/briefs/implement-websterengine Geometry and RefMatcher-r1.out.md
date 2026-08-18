All 3 declared cards in this batch (Cards 10, 11, 12) match 3 committed commits exactly. All cards complete — verified via honest count: 3 of 3 cards committed.

Summary of work:
- `/home/knatte/Code/loomyard/wts/webster-told-geometry/internal/websterengine/geometry.go` (new) — declares `Geometry` struct, Card 10.
- `/home/knatte/Code/loomyard/wts/webster-told-geometry/internal/websterengine/audit.go` — adds `RefMatcher` interface and `NeverMatches` type, Card 11.
- `/home/knatte/Code/loomyard/wts/webster-told-geometry/internal/websterengine/audit_test.go` — adds `TestNeverMatches_AlwaysFalse` and two compile-time `RefMatcher` satisfaction assertions, Card 12.

`verify: go test ./internal/websterengine/...` passes. Working tree clean (only the out-of-scope brief file remains untracked, as it did at session start).

{"status":"success","commit_sha":"63ae1baf0ef0e81ee766df04bd0481a00d8fda0d","session_id":"412ef7c9-741d-40e0-bf3a-369732a75526","cards_done":[10,11,12]}

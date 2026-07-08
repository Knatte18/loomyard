All three cards are implemented, committed, and verified.

Summary of work:
- Card 7 (already partially done from the interrupted attempt, reviewed and confirmed correct): `internal/perchengine/judge-circling-template.md`, `internal/perchengine/judge-milestone-template.md`, `internal/perchengine/triage-template.md`, `internal/perchengine/template.go`, `internal/perchengine/template_test.go` — committed as `perch: add judge (circling, milestone) and triage prompt templates` (9d5e63a).
- Card 8: `internal/perchengine/judgeverdict.go`, `internal/perchengine/judgeverdict_test.go` — `JudgeVerdict`/`TriageVerdict` types, framing-scoped `ParseJudgeVerdict`, `ParseTriageVerdict`, package-private `splitFrontmatter` — committed as `perch: add strict judge and triage verdict-file parsers` (f57a1e6).
- Card 9: `internal/perchengine/judge.go`, `internal/perchengine/judge_test.go`, `internal/perchengine/smoke_judge_test.go` — `Shuttle` seam, `judgeInputs`, fail-safe `runCircling`/`runMilestone`/`runTriage`, fake-shuttle unit tests, and an opt-in `-tags smoke` end-to-end test — committed as `perch: add fail-safe judge and triage spawners over the shuttle seam` (a631b9b).

One deliberate deviation from the batch's literal wording, noted in the file's header comment: `smoke_judge_test.go` is in package `perchengine` rather than `perchengine_test`, because `runCircling`/`judgeInputs` are unexported (the batch's own "external interface for batch 4" language confirms they're meant package-private, not exported). An external test package could not compile against them. I verified the smoke test actually runs and passes against a real `claude` binary in this environment (34s, real judge call parsed correctly), confirming the whole spawner chain works end-to-end, not just under fakes.

`go build ./...`, `go vet ./...`, and `go test ./internal/perchengine/` (the batch's `verify:`) all pass. Working tree is clean (`git status --porcelain --untracked-files=no` empty) except the untracked brief file itself, which is out of scope.

{"status":"success","commit_sha":"a631b9ba5450a11cb68168ce7308d8a8ed754d35","session_id":"4ff28e44-69b5-45b0-b3f7-7bac5d91666a"}

# Finding grades: Plan-Review perch

Code read at each run's base in the prime (`e5b56fe91` for intake, `d22189d84` for runs-overview).
Severity letters: B blocking, M medium, L low, N nit.

## twb-btf-is (intake-bugs-1012, arm s)

### 1. Round-1 dispositions

| Source | Sev | Title | Disposition | Right? |
|---|---|---|---|---|
| R1 F1 | M | Card 2 lets an argument outside the worktree root pass | fixed as proposed: clause dropped, outside-root result stated as missing, `go test ../outside` reporting row | right |
| R1 F2 | M | Card 2's scan-end branches have no reaching row | fixed as proposed: three no-report rows | wrong: `go -C .. test ./nope` reaches no scan-end branch (`goCommandDirAndPackages` returns `"", nil, false`, `verifymodule.go:226-228`, so no argument is read whether the scan ends or not); the report repeats the false premise "each would report `./nope` if the scan went on" |
| R1 F3 | L | Card 4's read error with the re-step spent has no outcome or test | fixed as proposed: bullet sentence plus subtest | right |
| own (gate) | - | `edit-test-file` gate findings on external test files | own change: Shared Decision rewritten to "named by glyph on their `_test` unit", cards 1 and 2 retargeted | right, class setup: the rule and the `_test` mapping exist only in the bench's installed `lyx`, not at `e5b56fe91`; inside the bench the claim holds |

### 2. Fork findings

None (arm s).

### 3. Round-2 findings

| Source | Sev | Title | Grade | Re-rated | Class | Cause |
|---|---|---|---|---|---|---|
| R2 F1 | L | Bound paragraph omits the Intent's exemptions (pipeline/`||` cd, unplaceable `-C`, Move bound, `--`) | real | L | A (scan-end part, KI5; Move bound and `--` parts are B) | execution: the fix's new rows pin passes the Bound sibling still omits |
| R2 F2 | L | Unplaceable `-C` row cannot tell scan end from skip; gate ok-false branch has no row | real | L | A (KI) | proposal |
| R2 F3 | L | Used-re-step subtest does not assert the read-error way forward | real | L | A | proposal: R1 F3's subtest spec listed the asserts without the way forward, applied as written |

### 4. Audit of fix.diff

No defect beyond those the round-2 review reports.
The gate own change is setup, not class A.

**Key:** KI R (the `go -C .. test ./nope` row and its premise kept); KI5 R (Bound unchanged).

**Tally:** round-1 wrong 1; fork findings 0/0/0; fork dispositions wrong 0; round-2 real/wrong/noise 3/0/0; class A by review 3 (proposal 2 / execution 1), by audit 0; class B 0; setup 0 (plus the setup-driven gate change).

## twb-btf-it (intake-bugs-1012, arm t)

### 1. Round-1 dispositions

| Source | Sev | Title | Disposition | Right? |
|---|---|---|---|---|
| R1 F1 | M | Card 2 lets an argument outside the worktree root pass | fixed differently: outside-root result is "missing without a stat", plus the `../outside` row | right: the evidence holds (a stat of the joined path could find a sibling of the worktree) |
| R1 F2 | M | Card 2's scan-end branches have no reaching row | fixed as proposed, plus a sentence "each of the four scan-end rows ... would report `./nope` if the scan went on" | wrong: the `-C` row reaches no branch, and the added sentence writes the false premise into the card |
| R1 F3 | L | Card 4's read error with the re-step spent | fixed as proposed, plus the way-forward and two-call assertions and the refusal-spec and doc.go twins | right |
| own (gate) | - | `edit-test-file` gate findings | own change as in arm s, Shared Decision adds "card 1 brings this repository's source to the same mapping" | right, class setup |

### 2. Fork findings

| Source | Sev | Title | Grade | Re-rated | Disposition | Right? |
|---|---|---|---|---|---|---|
| consistency 1 | L | Card 2's Bound omits the scan-end passes the new rows pin | real (KI5) | L | fixed: Bound lists pipeline/`||` cd and unplaceable `-C` | right |
| consistency 2 | L | Card 4's no-re-read stop reads against D5's literal "only" | noise (the card's reading is D5's; the fork says so) | N | fixed: one clause citing D5's single re-step | right (harmless) |
| claims | - | no findings | - | - | - | its checked list endorses the `-C` row's false premise ("a copy that ignored `ok` would join `./nope`"; the package list is nil) |
| way-forward | - | no findings | - | - | - | likewise endorses "each would report `./nope`" for the `-C` row |

### 3. Round-2 findings

| Source | Sev | Title | Grade | Re-rated | Class | Cause |
|---|---|---|---|---|---|---|
| R2 F1 | M | Guard branches unreached: gate ok-false, multi-field cd, `-C` row cannot tell end from skip | real | L | A (the `-C` part and line 82's sentence, KI); gate ok-false and multi-field cd parts B | proposal |
| R2 F2 | M | A `...` pattern not after a slash is reported and refused | real (valid Go pattern refused) | L | B | - |
| R2 F3 | L | Card 2 copies the chain walk instead of sharing it | noise (round 1 accepted the copy and asked for rows; preference) | N | B | - |
| R2 F4 | L | A cleared read error spends the re-step a later transient failure would get | real | L | B | - |
| R2 F5 | N | Re-step rows sit in the interrupted-child test | noise | N | B | - |

### 4. Audit of fix.diff

No defect beyond R2 F1.
The Bound rewrite, the D5 clause, the refusal-spec and doc.go twins and the way-forward assertion are sound.

**Key:** KI R (repeated and stated outright at line 82); KI5 A (fork: consistency 1).

**Tally:** round-1 wrong 1; fork findings real/wrong/noise 1/0/1; fork dispositions wrong 0; round-2 real/wrong/noise 3/0/2; class A by review 1 (proposal 1 / execution 0), by audit 0; class B 4; setup 0 (plus the setup-driven gate change).

## twb-btf-rs (runs-overview, arm s)

### 1. Round-1 dispositions

| Source | Sev | Title | Disposition | Right? |
|---|---|---|---|---|
| R1 F1 | B | Card 8's `NoticeLog.Now` paths missing from envkit `nilLegal` | fixed as proposed | right |
| R1 F2 | L | Card 1 says layout.go is untouched but edits `placement` | fixed as proposed | right |
| R1 F3 | L | Duration format planned twice | fixed as proposed: `statuspane.FormatMinutes`, with "no test of its own" | right on its premise (one symbol now); the proposal's no-test clause brings in KR (class A, below) |
| R1 F4 | L | `TestFitOverview` is a per-symbol test | fixed as proposed, budget 69 → 72 | right |

### 2. Fork findings

None (arm s).

### 3. Round-2 findings

| Source | Sev | Title | Grade | Re-rated | Class | Cause |
|---|---|---|---|---|---|---|
| R2 F1 | B | Plan rests on `lyx cleanup` (bc9ee7a02), absent at HEAD; budget bases off | true at bench base only | - | setup | - |
| R2 F2 | B | termwindow gains a tagged test file but no `TestMain` | true at bench base only: bc9ee7a02 adds `internal/termwindow/testmain_test.go` and makes every package carry one | - | setup | - |
| R2 F3 | M | Four `ValidateDisplay` refusals reach no test | real | M | B | - |
| R2 F4 | L | `orderStack`'s comment goes stale once top cells come first | real (`rules.go:51` passes `partitionByAnchor`'s output) | L | B | - |
| R2 F5 | L | `FormatMinutes`' hour and day forms covered by no named row | real (KR) | M | A | proposal: R1 F3's fix text carried the no-test clause, applied as proposed |

### 4. Audit of fix.diff

No defect beyond R2 F5.

**Key:** KR R (no-test clause kept; the review found it as F5).

**Tally:** round-1 wrong 0; fork findings 0/0/0; fork dispositions wrong 0; round-2 real/wrong/noise 3/0/0 (plus 2 setup); class A by review 1 (proposal 1 / execution 0), by audit 0; class B 2; setup 2.

## twb-btf-rt (runs-overview, arm t)

### 1. Round-1 dispositions

| Source | Sev | Title | Disposition | Right? |
|---|---|---|---|---|
| R1 F1 | B | envkit `nilLegal` paths | fixed as proposed | right |
| R1 F2 | L | layout.go wording | fixed as proposed | right |
| R1 F3 | L | Duration format planned twice | fixed as proposed, with the no-test clause | right on its premise; brings in KR (class A, audit) |
| R1 F4 | L | `TestFitOverview` per-symbol | fixed as proposed | right |

### 2. Fork findings

| Source | Sev | Title | Grade | Re-rated | Disposition | Right? |
|---|---|---|---|---|---|---|
| consistency 1 | L | Overview's card 5 summary line does not name the time format | noise (a card-list summary, not a contract) | N | fixed | right (harmless) |
| claims | - | no findings | - | - | - | its checked list endorses the false claim that the renderer tests cover `FormatMinutes` (no named row reaches an hour or a day) |
| way-forward | - | no findings | - | - | - | - |

### 3. Round-2 findings

| Source | Sev | Title | Grade | Re-rated | Class | Cause |
|---|---|---|---|---|---|---|
| R2 F1 | B | Card 8 claims the branch contains bc9ee7a02 | true at bench base only | - | setup | - |
| R2 F2 | M | battencli/orchcli budget bases do not match HEAD | true at bench base only (the +4 and +1 are bc9ee7a02's tests) | - | setup | - |

### 4. Audit of fix.diff

| Source | Title | Severity | Class | Cause |
|---|---|---|---|---|
| audit | Card 5's `FormatMinutes` "has no test of its own" while no named row of cards 7 or 9 reaches the `1h05m` or `2d03h` forms (KR) | M | A | proposal |

**Key:** KR R (no-test clause kept; the round-2 review missed it, the claims fork endorsed it).

**Tally:** round-1 wrong 0; fork findings real/wrong/noise 0/0/1; fork dispositions wrong 0; round-2 real/wrong/noise 0/0/0 (plus 2 setup); class A by review 0, by audit 1 (proposal 1 / execution 0); class B 0; setup 2.

## Rules adopted

- Real: a false statement about the code or plan, a prescribed branch no named row reaches, or wrong behaviour on valid input; noise: true but a preference or cosmetic, with no change a builder would need.
- A finding true only because the bench base lacks bc9ee7a02 or because the bench's installed `lyx` is newer than the base is class `setup`, kept out of real/wrong/noise.
- A fix that resolves its finding's premise is right even when the proposal brought in a new defect, which is counted as class A instead; a fix that leaves part of the premise standing is wrong.
- A round-2 finding mixing fix-introduced and pre-existing parts is classed A, with the B parts named in the row.
- A fork's "no findings" whose checked list endorses a false premise is noted but not counted as a finding.

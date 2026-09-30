// triage_test.go drives triageIntegrationFailure through a fake verifyRunner whose output depends on the fake bisector's current checkout,
// so classification is tested without spawning a process.

package websterengine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// triageFakeRepo records checkouts and restores and tracks the current checkout ("" is the branch head).
// ancestors holds every {sha, ref} pair IsAncestor answers true for; any other distinct pair answers false.
type triageFakeRepo struct {
	branch      string
	current     string
	checkouts   []string
	restores    []string
	restoreErr  error
	checkoutErr error
	ancestors   map[[2]string]bool
	ancestorErr error
}

func (r *triageFakeRepo) CurrentBranch() (string, error) { return r.branch, nil }

func (r *triageFakeRepo) CheckoutDetached(sha string) error {
	if r.checkoutErr != nil {
		return r.checkoutErr
	}
	r.checkouts = append(r.checkouts, sha)
	r.current = sha
	return nil
}

func (r *triageFakeRepo) RestoreBranch(ref string) error {
	r.restores = append(r.restores, ref)
	r.current = ""
	return r.restoreErr
}

func (r *triageFakeRepo) IsAncestor(sha, ref string) (bool, error) {
	if r.ancestorErr != nil {
		return false, r.ancestorErr
	}
	return sha == ref || r.ancestors[[2]string{sha, ref}], nil
}

// failOutput renders go test output failing the named tests in pkg, each at its subtest nesting depth.
func failOutput(pkg string, tests ...string) string {
	var b strings.Builder
	for _, name := range tests {
		indent := strings.Repeat("    ", strings.Count(name, "/"))
		b.WriteString(indent + "--- FAIL: " + name + " (0.00s)\n" + indent + "    x_test.go:1: boom " + name + "\n")
	}
	b.WriteString("FAIL\nFAIL\t" + pkg + "\t0.01s\n")
	return b.String()
}

// triageRig wires a fake runner keyed by the fake repo's checkout.
type triageRig struct {
	repo       *triageFakeRepo
	byCheckout map[string]verifyRun
	runErr     map[string]error
	logPaths   []string
	cmds       []string
	verifyCmd  string
	scratch    string
	firstLog   string
}

func newTriageRig(t *testing.T, firstLog string, head, baseline verifyRun) *triageRig {
	t.Helper()
	rig := &triageRig{
		repo:       &triageFakeRepo{branch: "main"},
		byCheckout: map[string]verifyRun{"": head, "base": baseline},
		runErr:     map[string]error{},
		verifyCmd:  "verify",
		scratch:    t.TempDir(),
	}
	rig.firstLog = filepath.Join(rig.scratch, "first.log")
	if firstLog != "" {
		if err := os.WriteFile(rig.firstLog, []byte(firstLog), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return rig
}

func (r *triageRig) run(cmd, worktree, logPath string) (verifyRun, error) {
	r.logPaths = append(r.logPaths, logPath)
	r.cmds = append(r.cmds, cmd)
	if err := r.runErr[r.repo.current]; err != nil {
		return verifyRun{}, err
	}
	return r.byCheckout[r.repo.current], nil
}

// triage runs triageIntegrationFailure with startSHAs as the batches' recorded start commits.
func (r *triageRig) triage(repo FabricBisector, startSHAs ...string) (triageOutcome, error) {
	return triageIntegrationFailure(r.run, repo, startSHAs, r.verifyCmd, "wt", r.scratch, r.firstLog)
}

func red(out string) verifyRun { return verifyRun{Passed: false, Output: out} }

var green = verifyRun{Passed: true}

func TestTriage_FlakyGreenRerun(t *testing.T) {
	rig := newTriageRig(t, failOutput("p/a", "TestA"), green, green)
	out, err := rig.triage(rig.repo, "base")
	if err != nil {
		t.Fatal(err)
	}
	if out.Triage.Verdict != TriageVerdictFlaky || out.Triage.Rerun != TriageRerunPassed {
		t.Errorf("triage = %+v; want flaky/passed", out.Triage)
	}
	if !reflect.DeepEqual(out.Triage.Flaky, []string{"p/a.TestA"}) {
		t.Errorf("Flaky = %v", out.Triage.Flaky)
	}
	if len(rig.repo.checkouts) != 0 || len(rig.logPaths) != 1 {
		t.Errorf("checkouts = %v, runs = %d; want no baseline run", rig.repo.checkouts, len(rig.logPaths))
	}
	if len(out.Failures) != 1 || !strings.Contains(out.Failures[0].Tail, "boom TestA") {
		t.Errorf("Failures = %+v; want first-run tail", out.Failures)
	}
}

func TestTriage_OpaqueGreenRerunIsFlaky(t *testing.T) {
	rig := newTriageRig(t, "vet: something\n", green, green)
	out, err := rig.triage(rig.repo, "base")
	if err != nil {
		t.Fatal(err)
	}
	if out.Triage.Verdict != TriageVerdictFlaky || !reflect.DeepEqual(out.Triage.Flaky, []string{opaqueFailureID}) {
		t.Errorf("triage = %+v; want flaky opaque", out.Triage)
	}
}

func TestTriage_MissingFirstLogGreenRerun(t *testing.T) {
	rig := newTriageRig(t, "", green, green)
	out, err := rig.triage(rig.repo, "base")
	if err != nil {
		t.Fatal(err)
	}
	if out.Triage.Verdict != TriageVerdictFlaky || !reflect.DeepEqual(out.Triage.Flaky, []string{opaqueFailureID}) {
		t.Errorf("triage = %+v; want flaky opaque", out.Triage)
	}
	if len(out.Failures) != 1 || out.Failures[0].Tail != "" {
		t.Errorf("Failures = %+v; want one opaque with empty tail", out.Failures)
	}
}

func TestTriage_PreExisting(t *testing.T) {
	o := failOutput("p/a", "TestA")
	rig := newTriageRig(t, o, red(o), red(o))
	out, err := rig.triage(rig.repo, "base")
	if err != nil {
		t.Fatal(err)
	}
	if out.Triage.Verdict != TriageVerdictPreExisting || out.Triage.Rerun != TriageRerunFailed || out.Triage.BaselineSHA != "base" {
		t.Errorf("triage = %+v", out.Triage)
	}
	if !reflect.DeepEqual(out.Triage.PreExisting, []string{"p/a.TestA"}) || len(out.Triage.Regressions) != 0 {
		t.Errorf("triage = %+v", out.Triage)
	}
	if !reflect.DeepEqual(rig.repo.checkouts, []string{"base"}) || !reflect.DeepEqual(rig.repo.restores, []string{"main"}) {
		t.Errorf("checkouts = %v restores = %v", rig.repo.checkouts, rig.repo.restores)
	}
}

func TestTriage_RegressionGreenAtBaseline(t *testing.T) {
	o := failOutput("p/a", "TestA")
	rig := newTriageRig(t, o, red(o), green)
	out, err := rig.triage(rig.repo, "base")
	if err != nil {
		t.Fatal(err)
	}
	if out.Triage.Verdict != TriageVerdictRegression || !reflect.DeepEqual(out.Triage.Regressions, []string{"p/a.TestA"}) {
		t.Errorf("triage = %+v", out.Triage)
	}
}

func TestTriage_AbsentAtBaselineIsRegression(t *testing.T) {
	rig := newTriageRig(t, failOutput("p/a", "TestA"), red(failOutput("p/a", "TestA")), red(failOutput("p/a", "TestOther")))
	out, err := rig.triage(rig.repo, "base")
	if err != nil {
		t.Fatal(err)
	}
	if out.Triage.Verdict != TriageVerdictRegression || !reflect.DeepEqual(out.Triage.Regressions, []string{"p/a.TestA"}) {
		t.Errorf("triage = %+v", out.Triage)
	}
}

func TestTriage_MixedYieldsRegressionWithBothLists(t *testing.T) {
	head := failOutput("p/a", "TestOld", "TestNew")
	rig := newTriageRig(t, head, red(head), red(failOutput("p/a", "TestOld")))
	out, err := rig.triage(rig.repo, "base")
	if err != nil {
		t.Fatal(err)
	}
	if out.Triage.Verdict != TriageVerdictRegression {
		t.Errorf("verdict = %q", out.Triage.Verdict)
	}
	if !reflect.DeepEqual(out.Triage.PreExisting, []string{"p/a.TestOld"}) || !reflect.DeepEqual(out.Triage.Regressions, []string{"p/a.TestNew"}) {
		t.Errorf("triage = %+v", out.Triage)
	}
}

func TestTriage_OpaqueRedIsRegressionEvenAtBaseline(t *testing.T) {
	rig := newTriageRig(t, "vet failed\n", red("vet failed\n"), red("vet failed\n"))
	out, err := rig.triage(rig.repo, "base")
	if err != nil {
		t.Fatal(err)
	}
	if out.Triage.Verdict != TriageVerdictRegression || !reflect.DeepEqual(out.Triage.Regressions, []string{opaqueFailureID}) {
		t.Errorf("triage = %+v", out.Triage)
	}
}

func TestTriage_NilRepo(t *testing.T) {
	o := failOutput("p/a", "TestA")
	t.Run("red rerun", func(t *testing.T) {
		rig := newTriageRig(t, o, red(o), red(o))
		out, err := rig.triage(nil, "base")
		if err != nil {
			t.Fatal(err)
		}
		if out.Triage.Verdict != TriageVerdictRegression || len(out.Warnings) != 1 || !strings.Contains(out.Warnings[0], "baseline") {
			t.Errorf("out = %+v; want regression plus baseline warning", out)
		}
		if out.Triage.BaselineSHA != "" {
			t.Errorf("BaselineSHA = %q; want empty", out.Triage.BaselineSHA)
		}
	})
	t.Run("green rerun", func(t *testing.T) {
		rig := newTriageRig(t, o, green, green)
		out, err := rig.triage(nil, "base")
		if err != nil {
			t.Fatal(err)
		}
		if out.Triage.Verdict != TriageVerdictFlaky || len(out.Warnings) != 0 {
			t.Errorf("out = %+v; want flaky, no warnings", out)
		}
	})
}

func TestTriage_NoStartSHAsBehavesLikeNilRepo(t *testing.T) {
	o := failOutput("p/a", "TestA")
	rig := newTriageRig(t, o, red(o), red(o))
	out, err := rig.triage(rig.repo)
	if err != nil {
		t.Fatal(err)
	}
	if out.Triage.Verdict != TriageVerdictRegression || len(out.Warnings) != 1 {
		t.Errorf("out = %+v", out)
	}
	if len(rig.repo.checkouts) != 0 {
		t.Errorf("checkouts = %v; want none", rig.repo.checkouts)
	}
}

func TestTriage_MissingFirstLogUsesRerunIdentities(t *testing.T) {
	o := failOutput("p/a", "TestA")
	rig := newTriageRig(t, "", red(o), green)
	out, err := rig.triage(rig.repo, "base")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out.Triage.Regressions, []string{"p/a.TestA"}) || len(out.Triage.Flaky) != 0 {
		t.Errorf("triage = %+v", out.Triage)
	}
}

func TestTriage_RerunOnlyAndFirstOnlyFailures(t *testing.T) {
	rig := newTriageRig(t, failOutput("p/a", "TestFirst"), red(failOutput("p/a", "TestRerun")), green)
	out, err := rig.triage(rig.repo, "base")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out.Triage.Flaky, []string{"p/a.TestFirst"}) {
		t.Errorf("Flaky = %v", out.Triage.Flaky)
	}
	if len(out.Failures) != 2 || out.Failures[0].ID != "p/a.TestRerun" || !strings.Contains(out.Failures[0].Tail, "boom TestRerun") ||
		out.Failures[1].ID != "p/a.TestFirst" || !strings.Contains(out.Failures[1].Tail, "boom TestFirst") {
		t.Errorf("Failures = %+v; want head set then first-only, each with its own tail", out.Failures)
	}
}

func TestTriage_RestoresBranchWhenBaselineRunErrors(t *testing.T) {
	o := failOutput("p/a", "TestA")
	rig := newTriageRig(t, o, red(o), green)
	boom := errors.New("spawn failed")
	rig.runErr["base"] = boom
	_, err := rig.triage(rig.repo, "base")
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v; want the run error", err)
	}
	if !reflect.DeepEqual(rig.repo.restores, []string{"main"}) {
		t.Errorf("restores = %v; want the branch restored", rig.repo.restores)
	}
}

func TestTriage_RestoreFailureIsReturned(t *testing.T) {
	o := failOutput("p/a", "TestA")
	rig := newTriageRig(t, o, red(o), red(o))
	rig.repo.restoreErr = errors.New("dirty tree")
	_, err := rig.triage(rig.repo, "base")
	if err == nil || !strings.Contains(err.Error(), "dirty tree") {
		t.Fatalf("err = %v; want the restore failure", err)
	}
}

func TestTriage_CheckoutErrorIsReturned(t *testing.T) {
	o := failOutput("p/a", "TestA")
	rig := newTriageRig(t, o, red(o), red(o))
	rig.repo.checkoutErr = errors.New("no such sha")
	_, err := rig.triage(rig.repo, "base")
	if err == nil || !strings.Contains(err.Error(), "no such sha") {
		t.Fatalf("err = %v; want the checkout error", err)
	}
}

func TestTriage_RerunSpawnErrorIsReturned(t *testing.T) {
	rig := newTriageRig(t, failOutput("p/a", "TestA"), green, green)
	rig.runErr[""] = errors.New("cannot spawn")
	if _, err := rig.triage(rig.repo, "base"); err == nil {
		t.Fatal("err = nil; want the spawn error")
	}
}

// TestTriage_BaselineIsEarliestStartSHA proves the baseline is the start commit every other start commit descends from,
// not the first one listed: a later batch that began first recorded the earliest start commit.
func TestTriage_BaselineIsEarliestStartSHA(t *testing.T) {
	o := failOutput("p/a", "TestA")
	rig := newTriageRig(t, o, red(o), red(o))
	rig.repo.ancestors = map[[2]string]bool{{"base", "late"}: true}
	out, err := rig.triage(rig.repo, "late", "base")
	if err != nil {
		t.Fatal(err)
	}
	if out.Triage.BaselineSHA != "base" || !reflect.DeepEqual(rig.repo.checkouts, []string{"base"}) {
		t.Errorf("BaselineSHA = %q, checkouts = %v; want the earliest start commit base", out.Triage.BaselineSHA, rig.repo.checkouts)
	}
	if out.Triage.Verdict != TriageVerdictPreExisting {
		t.Errorf("verdict = %q; want pre-existing", out.Triage.Verdict)
	}
}

// TestTriage_UnorderedStartSHAsFailClosed proves start commits with no single earliest member, or an ancestry probe that errors,
// skip the baseline run and classify every failure a regression with a warning.
func TestTriage_UnorderedStartSHAsFailClosed(t *testing.T) {
	tests := []struct {
		name        string
		ancestorErr error
	}{
		{"diverging start commits", nil},
		{"ancestry probe error", errors.New("merge-base failed")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := failOutput("p/a", "TestA")
			rig := newTriageRig(t, o, red(o), red(o))
			rig.repo.ancestorErr = tt.ancestorErr
			out, err := rig.triage(rig.repo, "left", "right")
			if err != nil {
				t.Fatal(err)
			}
			if out.Triage.Verdict != TriageVerdictRegression || !reflect.DeepEqual(out.Triage.Regressions, []string{"p/a.TestA"}) {
				t.Errorf("triage = %+v; want every failure a regression", out.Triage)
			}
			if len(out.Warnings) != 1 || !strings.Contains(out.Warnings[0], "no single earliest batch start commit") {
				t.Errorf("Warnings = %q; want the earliest-start-commit warning", out.Warnings)
			}
			if len(rig.repo.checkouts) != 0 || out.Triage.BaselineSHA != "" {
				t.Errorf("checkouts = %v, BaselineSHA = %q; want no baseline run", rig.repo.checkouts, out.Triage.BaselineSHA)
			}
		})
	}
}

func TestEarliestCommit(t *testing.T) {
	// a <- b <- c is one line of history; x diverges from it.
	ancestors := map[[2]string]bool{{"a", "b"}: true, {"a", "c"}: true, {"b", "c"}: true}
	tests := []struct {
		name    string
		shas    []string
		want    string
		wantErr bool
	}{
		{"single", []string{"b"}, "b", false},
		{"in order", []string{"a", "b", "c"}, "a", false},
		{"reversed", []string{"c", "b", "a"}, "a", false},
		{"earliest in the middle", []string{"c", "a", "b"}, "a", false},
		{"duplicates", []string{"b", "b", "c"}, "b", false},
		{"diverging", []string{"b", "x"}, "", true},
		{"diverging after a match", []string{"c", "a", "x"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := earliestCommit(&triageFakeRepo{ancestors: ancestors}, tt.shas)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("earliestCommit(%v) = %q, %v; want %q, error %v", tt.shas, got, err, tt.want, tt.wantErr)
			}
		})
	}
}

// TestTriage_SiblingSubtestIsRegression proves a failing subtest is not excused by a different subtest of the same test failing at baseline.
func TestTriage_SiblingSubtestIsRegression(t *testing.T) {
	head := failOutput("p/a", "TestX", "TestX/b")
	rig := newTriageRig(t, head, red(head), red(failOutput("p/a", "TestX", "TestX/a")))
	out, err := rig.triage(rig.repo, "base")
	if err != nil {
		t.Fatal(err)
	}
	if out.Triage.Verdict != TriageVerdictRegression || !reflect.DeepEqual(out.Triage.Regressions, []string{"p/a.TestX/b"}) {
		t.Errorf("triage = %+v; want regression p/a.TestX/b", out.Triage)
	}
}

// TestTriage_PackageIdentityIsNeverPreExisting proves a package identity failing identically at baseline is still a regression,
// alongside a test identity that is excused.
func TestTriage_PackageIdentityIsNeverPreExisting(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   IntegrationTriage
	}{
		{
			name:   "build failure",
			output: "# p/b\np/b/b.go:3: undefined: x\nFAIL\tp/b [build failed]\n" + failOutput("p/a", "TestA"),
			want:   IntegrationTriage{PreExisting: []string{"p/a.TestA"}, Regressions: []string{"p/b"}},
		},
		{
			name:   "panic after a failing test",
			output: "--- FAIL: TestA (0.00s)\npanic: boom [recovered]\n\ngoroutine 7 [running]:\nFAIL\tp/a\t0.01s\n",
			want:   IntegrationTriage{PreExisting: []string{"p/a.TestA"}, Regressions: []string{"p/a"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rig := newTriageRig(t, tt.output, red(tt.output), red(tt.output))
			out, err := rig.triage(rig.repo, "base")
			if err != nil {
				t.Fatal(err)
			}
			if out.Triage.Verdict != TriageVerdictRegression ||
				!reflect.DeepEqual(out.Triage.PreExisting, tt.want.PreExisting) || !reflect.DeepEqual(out.Triage.Regressions, tt.want.Regressions) {
				t.Errorf("triage = %+v; want regression with pre-existing %v and regressions %v", out.Triage, tt.want.PreExisting, tt.want.Regressions)
			}
		})
	}
}

// TestTriage_PackageIdentityGreenRerunIsFlaky proves a package identity is still excused as flaky when the rerun passes cleanly.
func TestTriage_PackageIdentityGreenRerunIsFlaky(t *testing.T) {
	rig := newTriageRig(t, "# p/b\np/b/b.go:3: undefined: x\nFAIL\tp/b [build failed]\n", green, green)
	out, err := rig.triage(rig.repo, "base")
	if err != nil {
		t.Fatal(err)
	}
	if out.Triage.Verdict != TriageVerdictFlaky || !reflect.DeepEqual(out.Triage.Flaky, []string{"p/b"}) {
		t.Errorf("triage = %+v; want flaky p/b", out.Triage)
	}
}

// TestTriage_UnprovableVerifyChainFailsClosed proves a verify command whose red exit a non-test step could explain skips the baseline run:
// the rerun runs the command unchanged, and every failure is a regression with a warning.
func TestTriage_UnprovableVerifyChainFailsClosed(t *testing.T) {
	tests := []struct {
		name      string
		verifyCmd string
	}{
		{"semicolon masks the vet exit", "go vet ./... ; go test ./..."},
		{"pipe masks the test exit", "go test ./... | tee log"},
		{"or masks the vet exit", "go vet ./... || go test ./..."},
		{"failfast leaves tests unrun", "go test -failfast ./..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := failOutput("p/a", "TestA")
			rig := newTriageRig(t, o, red(o), red(o))
			rig.verifyCmd = tt.verifyCmd
			out, err := rig.triage(rig.repo, "base")
			if err != nil {
				t.Fatal(err)
			}
			if out.Triage.Verdict != TriageVerdictRegression || !reflect.DeepEqual(out.Triage.Regressions, []string{"p/a.TestA"}) {
				t.Errorf("triage = %+v; want every failure a regression", out.Triage)
			}
			if len(out.Warnings) != 1 || !strings.Contains(out.Warnings[0], "every failure is treated as a regression") {
				t.Errorf("Warnings = %q; want one fail-closed warning", out.Warnings)
			}
			if len(rig.repo.checkouts) != 0 || !reflect.DeepEqual(rig.cmds, []string{tt.verifyCmd}) {
				t.Errorf("checkouts = %v, cmds = %q; want only the unchanged rerun", rig.repo.checkouts, rig.cmds)
			}
		})
	}
}

// TestTriage_VerifyChainStoppedBeforeLastStepFailsClosed proves a pre-existing failure in an early && step is not excused:
// the chain stopped there, so its later steps never ran at head.
func TestTriage_VerifyChainStoppedBeforeLastStepFailsClosed(t *testing.T) {
	o := failOutput("p/a", "TestA")
	rig := newTriageRig(t, o, red(o), red(o))
	rig.verifyCmd = "go test ./a/... && go test ./b/..."
	out, err := rig.triage(rig.repo, "base")
	if err != nil {
		t.Fatal(err)
	}
	if out.Triage.Verdict != TriageVerdictRegression || len(out.Triage.PreExisting) != 0 {
		t.Errorf("triage = %+v; want every failure a regression", out.Triage)
	}
	if len(out.Warnings) != 1 || !strings.Contains(out.Warnings[0], "stopped before its last step") {
		t.Errorf("Warnings = %q; want the stopped-chain warning", out.Warnings)
	}
	if len(rig.repo.checkouts) != 0 {
		t.Errorf("checkouts = %v; want no baseline run", rig.repo.checkouts)
	}
}

// TestTriage_VerifyChainReachingLastStepComparesBaseline proves a red && chain whose rerun reached its last step is compared against the baseline:
// the rerun runs the marker-instrumented chain, and the baseline runs the command unchanged.
func TestTriage_VerifyChainReachingLastStepComparesBaseline(t *testing.T) {
	o := failOutput("p/a", "TestA")
	rig := newTriageRig(t, o, red(verifyChainMarker+"\n"+o), red(o))
	rig.verifyCmd = "go vet ./... && go test ./..."
	out, err := rig.triage(rig.repo, "base")
	if err != nil {
		t.Fatal(err)
	}
	if out.Triage.Verdict != TriageVerdictPreExisting || !reflect.DeepEqual(out.Triage.PreExisting, []string{"p/a.TestA"}) {
		t.Errorf("triage = %+v; want pre-existing p/a.TestA", out.Triage)
	}
	want := []string{"go vet ./... && echo " + verifyChainMarker + " && go test ./...", rig.verifyCmd}
	if !reflect.DeepEqual(rig.cmds, want) {
		t.Errorf("cmds = %q; want %q", rig.cmds, want)
	}
}

func TestInstrumentVerifyChain(t *testing.T) {
	tests := []struct {
		name       string
		verifyCmd  string
		want       string
		wantReason bool
	}{
		{"single step is unchanged", "go test ./...", "go test ./...", false},
		{"marker precedes the last step", "go vet ./... && go build ./... && go test ./...", "go vet ./... && go build ./... && echo " + verifyChainMarker + " && go test ./...", false},
		{"semicolon", "go vet ./...; go test ./...", "", true},
		{"pipe", "go test ./... | tee out", "", true},
		{"or", "go vet ./... || true", "", true},
		{"background ampersand", "go vet ./... & go test ./...", "", true},
		{"stderr redirect", "go test ./... 2>&1", "", true},
		{"subshell", "(go test ./...)", "", true},
		{"command substitution", "go test $(go list ./...)", "", true},
		{"quoted ampersands", "go test -run 'A&&B' ./...", "", true},
		{"negation", "! go test ./...", "", true},
		{"inline comment", "go test ./... # all", "", true},
		{"empty step", "go vet ./... && && go test ./...", "", true},
		{"failfast flag", "go test -failfast ./...", "", true},
		{"failfast with value", "go test -failfast=true ./...", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := instrumentVerifyChain(tt.verifyCmd)
			if got != tt.want || (reason != "") != tt.wantReason {
				t.Errorf("instrumentVerifyChain(%q) = %q, %q; want %q, reason %v", tt.verifyCmd, got, reason, tt.want, tt.wantReason)
			}
		})
	}
}

func TestTriageWarnings(t *testing.T) {
	flaky := IntegrationTriage{Flaky: []string{"p.A", "p.B"}}
	pre := IntegrationTriage{PreExisting: []string{"p.C"}, BaselineSHA: "abc"}
	both := IntegrationTriage{Flaky: flaky.Flaky, PreExisting: pre.PreExisting, BaselineSHA: "abc", Regressions: []string{"p.R"}}
	cases := []struct {
		name string
		in   IntegrationTriage
		want []string
	}{
		{"flaky only", flaky, []string{"integration verify: flaky failures passed or vanished on rerun: p.A, p.B"}},
		{"pre-existing only", pre, []string{"integration verify: pre-existing failures also fail at baseline abc: p.C"}},
		{"both", both, []string{
			"integration verify: flaky failures passed or vanished on rerun: p.A, p.B",
			"integration verify: pre-existing failures also fail at baseline abc: p.C",
		}},
		{"neither", IntegrationTriage{Regressions: []string{"p.R"}}, nil},
	}
	for _, c := range cases {
		if got := triageWarnings(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q; want %q", c.name, got, c.want)
		}
	}
}

func TestTriageStuckReason(t *testing.T) {
	regs := []IntegrationFailure{{ID: "p.A"}, {ID: "p.B"}}
	want := "integration verify regressed: p.A, p.B (offending card: 3-foo)"
	if got := triageStuckReason(regs, "3-foo"); got != want {
		t.Errorf("got %q; want %q", got, want)
	}
	want = "integration verify regressed: p.A, p.B (offending card not localized)"
	if got := triageStuckReason(regs, "unknown"); got != want {
		t.Errorf("got %q; want %q", got, want)
	}
}

func TestWriteTriageFrictionNote(t *testing.T) {
	t.Run("content", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "friction")
		tr := IntegrationTriage{Flaky: []string{"p.A"}, PreExisting: []string{"p.B", "p.C"}}
		if err := writeTriageFrictionNote(dir, tr); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(dir, "webster-verify-triage.md"))
		if err != nil {
			t.Fatal(err)
		}
		want := "Integration-suite triage: non-regression verify failures\n\n" +
			"These identities failed the first verify run and passed or vanished on rerun (flaky):\n\n- p.A\n\n" +
			"These identities also fail at the plan's starting commit (pre-existing):\n\n- p.B\n- p.C\n\n"
		if string(got) != want {
			t.Errorf("note = %q; want %q", got, want)
		}
	})
	t.Run("empty dir", func(t *testing.T) {
		cwd := t.TempDir()
		t.Chdir(cwd)
		if err := writeTriageFrictionNote("", IntegrationTriage{Flaky: []string{"p.A"}}); err != nil {
			t.Fatal(err)
		}
		if entries, _ := os.ReadDir(cwd); len(entries) != 0 {
			t.Errorf("wrote %d entries; want none", len(entries))
		}
	})
	t.Run("regression only", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "friction")
		if err := writeTriageFrictionNote(dir, IntegrationTriage{Regressions: []string{"p.R"}}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("friction dir exists (err = %v); want nothing written", err)
		}
	})
}

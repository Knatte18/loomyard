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
type triageFakeRepo struct {
	branch      string
	current     string
	checkouts   []string
	restores    []string
	restoreErr  error
	checkoutErr error
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

// failOutput renders go test output failing the named top-level tests in pkg.
func failOutput(pkg string, tests ...string) string {
	var b strings.Builder
	for _, name := range tests {
		b.WriteString("--- FAIL: " + name + " (0.00s)\n    x_test.go:1: boom " + name + "\n")
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
	scratch    string
	firstLog   string
}

func newTriageRig(t *testing.T, firstLog string, head, baseline verifyRun) *triageRig {
	t.Helper()
	rig := &triageRig{
		repo:       &triageFakeRepo{branch: "main"},
		byCheckout: map[string]verifyRun{"": head, "base": baseline},
		runErr:     map[string]error{},
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
	if err := r.runErr[r.repo.current]; err != nil {
		return verifyRun{}, err
	}
	return r.byCheckout[r.repo.current], nil
}

func (r *triageRig) triage(repo FabricBisector, baseline string) (triageOutcome, error) {
	return triageIntegrationFailure(r.run, repo, baseline, "verify", "wt", r.scratch, r.firstLog)
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

func TestTriage_EmptyBaselineSHABehavesLikeNilRepo(t *testing.T) {
	o := failOutput("p/a", "TestA")
	rig := newTriageRig(t, o, red(o), red(o))
	out, err := rig.triage(rig.repo, "")
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

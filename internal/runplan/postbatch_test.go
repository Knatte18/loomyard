// postbatch_test.go covers how the post-batch checks treat a Create target the decision record drops.
// Tier 0: a fake index and a decision record in a temp directory.

package runplan

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// errPassReachedRebind is what the fake index's RebindResigns returns, so a test sees the pass get past the done-checks.
var errPassReachedRebind = errors.New("pass reached the re-sign rebind")

// errPassReachedBind is what the recording delta's BindHandles returns, so a test sees the pass reach handle binding.
var errPassReachedBind = errors.New("pass reached handle binding")

// doneCheckFakeIndex reports every Create target of the cards it is asked about as not done.
// It ends the pass at the re-sign rebind.
type doneCheckFakeIndex struct {
	planindex.Index
}

func (doneCheckFakeIndex) DoneChecks(_ *planparser.Plan, cards []planparser.Card, _ string) ([]planindex.Finding, error) {
	var findings []planindex.Finding
	for _, c := range cards {
		for _, g := range c.TargetGroups {
			if g.Type != planparser.CardTypeCreate {
				continue
			}
			for _, ref := range g.Refs {
				findings = append(findings, planindex.Finding{Card: c.ID(), Check: "create-not-done", Severity: planindex.SeverityBlocking, Detail: ref})
			}
		}
	}
	return findings, nil
}

func (doneCheckFakeIndex) RebindResigns(*planparser.Plan, []planparser.Card, string) ([]planindex.Finding, error) {
	return nil, errPassReachedRebind
}

// bindFakeIndex finds every card done and ends the pass at handle binding, recording the cards binding was asked about.
type bindFakeIndex struct {
	planindex.Index
	bound *[]planparser.Card
}

func (bindFakeIndex) DoneChecks(*planparser.Plan, []planparser.Card, string) ([]planindex.Finding, error) {
	return nil, nil
}

func (bindFakeIndex) RebindResigns(*planparser.Plan, []planparser.Card, string) ([]planindex.Finding, error) {
	return nil, nil
}

func (i bindFakeIndex) Delta(string, string, string) (planindex.Delta, error) {
	return recordingDelta{bound: i.bound}, nil
}

type recordingDelta struct {
	planindex.Delta
	bound *[]planparser.Card
}

func (d recordingDelta) BindHandles(_ *planparser.Plan, _ string, cards []planparser.Card) ([]planindex.Finding, []planindex.BindOutcome, error) {
	*d.bound = cards
	return nil, nil, errPassReachedBind
}

// writeDroppingRecord writes a decision record whose one added entry, titled "Drop it", drops each of targets.
func writeDroppingRecord(t *testing.T, targets ...string) string {
	t.Helper()
	text := "## Decisions\n\n### Added after Discussion (parent, 2026-11-21): Drop it\n\nDecision: drop them.\nRationale: unneeded.\n"
	for _, target := range targets {
		text += "Drops Create: `" + target + "`\n"
	}
	record := filepath.Join(t.TempDir(), "decision.md")
	if err := os.WriteFile(record, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return record
}

func TestPostBatchChecks_DroppedCreateSkipsItsDoneCheck(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const ref = "internal/foo/foo.go"
	const secondRef = "internal/foo/bar.go"
	card := planparser.Card{
		Number:       1,
		Slug:         "foo",
		TargetGroups: []planparser.TargetGroup{{Type: planparser.CardTypeCreate, Refs: []string{ref, secondRef}}},
	}
	inputs := func(recordPath string) PostBatchInputs {
		return PostBatchInputs{
			Geom:               Geometry{Index: doneCheckFakeIndex{}, PlanDir: t.TempDir()},
			Number:             1,
			Cards:              []planparser.Card{card},
			DecisionRecordPath: recordPath,
		}
	}
	warningFor := func(target string) string {
		return `card 1-foo: Create target "` + target + `" is dropped by the decision "Drop it", so its done-check did not run`
	}

	_, _, err := PostBatchChecks(inputs(""))
	if !errors.Is(err, ErrCardNotDone) {
		t.Fatalf("PostBatchChecks without a decision record: err = %v; want ErrCardNotDone", err)
	}

	t.Run("DroppingEveryTargetUnblocksTheCard", func(t *testing.T) {
		t.Parallel()
		_, warnings, err := PostBatchChecks(inputs(writeDroppingRecord(t, ref, secondRef)))
		if errors.Is(err, ErrCardNotDone) {
			t.Fatalf("PostBatchChecks with both targets dropped: err = %v; want no ErrCardNotDone", err)
		}
		if !errors.Is(err, errPassReachedRebind) {
			t.Errorf("PostBatchChecks with both targets dropped: err = %v; want the pass to reach the re-sign rebind", err)
		}
		for _, target := range []string{ref, secondRef} {
			if !slices.Contains(warnings, warningFor(target)) {
				t.Errorf("PostBatchChecks with both targets dropped: warnings = %q; want %q among them", warnings, warningFor(target))
			}
		}
		if len(card.TargetGroups[0].Refs) != 2 {
			t.Errorf("caller's card refs = %q; want both left in place", card.TargetGroups[0].Refs)
		}
	})

	t.Run("DroppingOneTargetKeepsTheOtherBlocking", func(t *testing.T) {
		t.Parallel()
		_, warnings, err := PostBatchChecks(inputs(writeDroppingRecord(t, ref)))
		if !errors.Is(err, ErrCardNotDone) {
			t.Fatalf("PostBatchChecks with one target dropped: err = %v; want ErrCardNotDone", err)
		}
		if !strings.Contains(err.Error(), secondRef) || strings.Contains(err.Error(), ref) {
			t.Errorf("PostBatchChecks with one target dropped: err = %q; want it to name only %q", err, secondRef)
		}
		if want := []string{warningFor(ref)}; !slices.Equal(warnings, want) {
			t.Errorf("PostBatchChecks with one target dropped: warnings = %q; want %q", warnings, want)
		}
	})
}

// TestPostBatchChecks_DroppedHandleDeclarationIsNotBound proves a dropped Create declared through a plan: handle.
// The card owns no handle for it, so binding is never asked about it.
// Its sibling handle stays bound, so the pass never reports the unmatched handle as a bind-count mismatch.
func TestPostBatchChecks_DroppedHandleDeclarationIsNotBound(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const droppedHandle = "plan:internal/foo#Dropped"
	const keptHandle = "plan:internal/foo#Kept"
	declarations := []planparser.CardDeclaration{{Handle: droppedHandle, Decl: "func Dropped()"}, {Handle: keptHandle, Decl: "func Kept()"}}
	card := planparser.Card{
		Number:       1,
		Slug:         "foo",
		Declarations: declarations,
		TargetGroups: []planparser.TargetGroup{{Type: planparser.CardTypeCreate, Refs: []string{droppedHandle, keptHandle}, Declarations: declarations}},
	}
	var bound []planparser.Card

	_, _, err := PostBatchChecks(PostBatchInputs{
		State:              &State{},
		Geom:               Geometry{Index: bindFakeIndex{bound: &bound}, PlanDir: t.TempDir(), RunDir: t.TempDir()},
		Number:             1,
		Cards:              []planparser.Card{card},
		DecisionRecordPath: writeDroppingRecord(t, droppedHandle),
	})

	if !errors.Is(err, errPassReachedBind) {
		t.Fatalf("PostBatchChecks err = %v; want the pass to reach handle binding", err)
	}
	if len(bound) != 1 {
		t.Fatalf("binding was asked about %d cards; want 1", len(bound))
	}
	if got := planparser.OwnedHandles(bound[0]); !slices.Equal(got, []string{keptHandle}) {
		t.Errorf("handles binding was asked about = %q; want only %q", got, keptHandle)
	}
	if len(card.Declarations) != 2 || len(card.TargetGroups[0].Declarations) != 2 {
		t.Errorf("caller's card declarations = %d and %d; want both left in place", len(card.Declarations), len(card.TargetGroups[0].Declarations))
	}
}

func TestPostBatchChecks_UnreadableDecisionRecordNamesTheWayForward(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// A directory in the record's place fails the read with an error that is not a missing file.
	record := t.TempDir()

	_, _, err := PostBatchChecks(PostBatchInputs{
		Geom:               Geometry{Index: doneCheckFakeIndex{}, PlanDir: t.TempDir()},
		Number:             1,
		Cards:              []planparser.Card{{Number: 1, Slug: "foo"}},
		DecisionRecordPath: record,
	})

	if err == nil || errors.Is(err, ErrCardNotDone) {
		t.Fatalf("PostBatchChecks err = %v; want a read error that is no ErrCardNotDone", err)
	}
	for _, want := range []string{record, "way forward: ", "lym loom decision replace"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("PostBatchChecks err = %q; want it to contain %q", err, want)
		}
	}
}

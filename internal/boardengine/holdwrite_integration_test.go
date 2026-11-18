// holdwrite_integration_test.go covers WriteHold over a real board repository and a bare origin.
// It adds a second clone for the hold another machine pushes first.

package boardengine

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// requireHoldRefusal returns err as the *HoldRefusal it must be.
func requireHoldRefusal(t *testing.T, err error) *HoldRefusal {
	t.Helper()
	var refusal *HoldRefusal
	if !errors.As(err, &refusal) {
		t.Fatalf("error = %v; want a *HoldRefusal", err)
	}
	return refusal
}

func TestWriteAdopt(t *testing.T) {
	tiergate.Require(t, 1)
	// Not parallel at the top: registering the replayer is process-global.
	// The rows that set holdSetterBeforePushHook, also process-global, run serially before the parallel rows start.
	fabricengine.RegisterBoltReplayer(FabricReplay{})

	at := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	here := machinename.Identity{ID: "id-a", Name: "neve"}
	there := machinename.Identity{ID: "id-b", Name: "osl46"}
	adopt := func(identity machinename.Identity, slugs ...string) AdoptRequest {
		return AdoptRequest{Machine: identity.Name, Identity: identity, Slugs: slugs, Now: at, ParentOf: parentOfBoxed}
	}
	takeHold := func(identity machinename.Identity, slugs ...string) AdoptRequest {
		req := adopt(identity, slugs...)
		req.TakeHold = true
		return req
	}
	// seed writes hold from the second clone, so it stands on origin before the first clone's write.
	seed := func(t *testing.T, f *ownerFixture, hold Hold) {
		t.Helper()
		if got, err := WriteHold(f.otherDir, hold); err != nil || !got.Wrote {
			t.Fatalf("seed WriteHold() = %+v, %v; want Wrote", got, err)
		}
	}
	thereHold := func(subjects ...string) Hold {
		hold := Hold{Kind: HoldKindAdopt, Machine: there.Name, At: at}
		for _, subject := range subjects {
			hold.Subjects = append(hold.Subjects, HoldSubject{Subject: subject, PriorOwner: "old"})
		}
		return hold
	}
	// planted is a hook that runs write from the second clone once, with the hook itself cleared first.
	planted := func(t *testing.T, write func()) {
		t.Helper()
		holdSetterBeforePushHook = func() {
			holdSetterBeforePushHook = nil
			write()
		}
		t.Cleanup(func() { holdSetterBeforePushHook = nil })
	}
	mustAdopt := func(t *testing.T, f *ownerFixture, req AdoptRequest) AdoptWrite {
		t.Helper()
		got, err := WriteAdopt(f.boardDir, req)
		if err != nil {
			t.Fatalf("WriteAdopt() error = %v; want nil", err)
		}
		return got
	}
	wantOriginHolder := func(t *testing.T, f *ownerFixture, machine string) {
		t.Helper()
		holds := f.originOwners(t).Holds
		if len(holds) != 1 || holds[0].Machine != machine {
			t.Errorf("origin holds = %+v; want the one hold of %s", holds, machine)
		}
	}

	t.Run("NoHoldWritesTheHoldTheOwnersAndTheRegistryInOneCommit", func(t *testing.T) {
		t.Parallel()
		f := newOwnerFixture(t, "neve")

		got := mustAdopt(t, f, adopt(here, "boxed", "beamer"))
		wantSubjects := []HoldSubject{{Subject: "boxed", PriorOwner: "box"}, {Subject: "beamer", PriorOwner: "neve"}}
		if got.Kept || got.Hold.Machine != "neve" || got.Hold.Kind != HoldKindAdopt || !slices.Equal(got.Hold.Subjects, wantSubjects) {
			t.Errorf("WriteAdopt() Hold = %+v, Kept %v; want a new adopt hold of neve over %v", got.Hold, got.Kept, wantSubjects)
		}
		if !slices.Equal(got.Recorded, []string{"boxed", "beamer"}) {
			t.Errorf("WriteAdopt() Recorded = %v; want [boxed beamer]", got.Recorded)
		}
		origin := f.originOwners(t)
		if origin.Entries["boxed"].Owner != "neve" || origin.Entries["beamer"].Owner != "neve" {
			t.Errorf("origin owners = boxed %q, beamer %q; want neve for both", origin.Entries["boxed"].Owner, origin.Entries["beamer"].Owner)
		}
		if len(origin.Holds) != 1 || !slices.Equal(origin.Holds[0].Subjects, wantSubjects) {
			t.Errorf("origin holds = %+v; want the one hold over %v", origin.Holds, wantSubjects)
		}
		if want := (Registry{"id-a": {Name: "neve"}}); !reflect.DeepEqual(origin.Registry, want) {
			t.Errorf("origin registry = %v; want %v", origin.Registry, want)
		}
		if count := gitkit.Git(t, f.bare, "rev-list", "--count", f.seedHead+"..main"); count != "1" {
			t.Errorf("commits on origin since the seed = %s; want 1, the hold, the owners and the registry in one commit", count)
		}
	})

	t.Run("ThisMachinesSameSetHoldStaysWithItsKindFixTaskAndTries", func(t *testing.T) {
		t.Parallel()
		f := newOwnerFixture(t, "neve")
		fixHold := Hold{
			Kind: HoldKindAdoptFix, Machine: "neve", At: at, FixTask: "fix-1", FixMachine: "neve",
			Subjects: []HoldSubject{{Subject: "beamer", PriorOwner: "neve", Tries: 2}, {Subject: "boxed", PriorOwner: "box"}},
		}
		if got, err := WriteHold(f.boardDir, fixHold); err != nil || !got.Wrote {
			t.Fatalf("WriteHold() = %+v, %v; want Wrote", got, err)
		}

		got := mustAdopt(t, f, adopt(here, "boxed", "beamer"))
		if !got.Kept || got.Hold.Kind != HoldKindAdoptFix || got.Hold.FixTask != "fix-1" || !slices.Equal(got.Hold.Subjects, fixHold.Subjects) {
			t.Errorf("WriteAdopt() = %+v; want the standing adopt-fix hold kept with its fix task and tries", got)
		}
		origin := f.originOwners(t)
		if len(origin.Holds) != 1 || !slices.Equal(origin.Holds[0].Subjects, fixHold.Subjects) || origin.Holds[0].FixTask != "fix-1" {
			t.Errorf("origin holds = %+v; want the standing hold untouched", origin.Holds)
		}
		if origin.Entries["boxed"].Owner != "neve" || origin.Entries["beamer"].Owner != "neve" {
			t.Errorf("origin owners = boxed %q, beamer %q; want neve for both", origin.Entries["boxed"].Owner, origin.Entries["beamer"].Owner)
		}
	})

	otherSets := []struct {
		name  string
		held  []string
		adopt []string
	}{
		{"Overlapping", []string{"beamer", "boxed"}, []string{"beamer"}},
		{"Disjoint", []string{"boxed"}, []string{"beamer"}},
	}
	for _, tt := range otherSets {
		t.Run("ThisMachinesOtherSetHoldRefusesNamingItsSubjects_"+tt.name, func(t *testing.T) {
			t.Parallel()
			f := newOwnerFixture(t, "neve")
			hold := thereHold(tt.held...)
			hold.Machine = "neve"
			if got, err := WriteHold(f.boardDir, hold); err != nil || !got.Wrote {
				t.Fatalf("WriteHold() = %+v, %v; want Wrote", got, err)
			}
			head := gitkit.RevParse(t, f.bare, "main")

			_, err := WriteAdopt(f.boardDir, adopt(here, tt.adopt...))
			refusal := requireHoldRefusal(t, err)
			if want := "`lym adopt " + strings.Join(tt.held, " ") + "` first"; !strings.Contains(refusal.Error(), want) {
				t.Errorf("WriteAdopt() error = %v; want the way forward %q", refusal, want)
			}
			if after := gitkit.RevParse(t, f.bare, "main"); after != head {
				t.Errorf("origin head = %s after a refusal; want it to stay %s", after, head)
			}
		})
	}

	t.Run("AnotherMachinesHoldRefusesNamingItsMachineAndTimeAndNoTakeHoldFlag", func(t *testing.T) {
		t.Parallel()
		f := newOwnerFixture(t, "neve")
		seed(t, f, thereHold("beamer"))
		head := gitkit.RevParse(t, f.bare, "main")

		_, err := WriteAdopt(f.boardDir, adopt(here, "beamer"))
		refusal := requireHoldRefusal(t, err)
		if refusal.Machine != "osl46" || !strings.Contains(refusal.Error(), "osl46") || !strings.Contains(refusal.Error(), "2026-11-17T09:00:00Z") {
			t.Errorf("WriteAdopt() error = %v; want it to name osl46 and the hold's time", refusal)
		}
		if strings.Contains(refusal.Error(), "take-hold") {
			t.Errorf("WriteAdopt() error = %v; want no mention of the take-hold flag", refusal)
		}
		if after := gitkit.RevParse(t, f.bare, "main"); after != head {
			t.Errorf("origin head = %s after a refusal; want it to stay %s", after, head)
		}
	})

	t.Run("TakeHoldRewritesTheHoldToThisMachineKeepingKindTaskTriesAndPriorOwners", func(t *testing.T) {
		t.Parallel()
		f := newOwnerFixture(t, "neve")
		fixHold := thereHold("beamer")
		fixHold.Kind, fixHold.FixTask = HoldKindAdoptFix, "fix-1"
		fixHold.Subjects[0].Tries = 1
		seed(t, f, fixHold)

		got := mustAdopt(t, f, takeHold(here, "beamer"))
		want := []HoldSubject{{Subject: "beamer", PriorOwner: "old", Tries: 1}}
		if got.Hold.Machine != "neve" || got.Hold.Kind != HoldKindAdoptFix || got.Hold.FixTask != "fix-1" ||
			got.Hold.FixMachine != "osl46" || !slices.Equal(got.Hold.Subjects, want) {
			t.Errorf("WriteAdopt() Hold = %+v; want neve's adopt-fix hold of fix-1 filed by osl46 over %v", got.Hold, want)
		}
		wantOriginHolder(t, f, "neve")
		if owner := f.originOwners(t).Entries["beamer"].Owner; owner != "neve" {
			t.Errorf("origin owner of beamer = %q; want neve", owner)
		}
	})

	t.Run("TakeHoldOverAnotherSetRefusesNamingTheHoldsSubjectsAndTheTakeHoldRerun", func(t *testing.T) {
		t.Parallel()
		f := newOwnerFixture(t, "neve")
		seed(t, f, thereHold("beamer", "boxed"))

		_, err := WriteAdopt(f.boardDir, takeHold(here, "beamer"))
		refusal := requireHoldRefusal(t, err)
		if refusal.Machine != "osl46" || !strings.Contains(refusal.Error(), "lym adopt --take-hold beamer boxed") {
			t.Errorf("WriteAdopt() error = %v; want it to name osl46 and `lym adopt --take-hold beamer boxed`", refusal)
		}
		wantOriginHolder(t, f, "osl46")
	})

	t.Run("TwoClonesAdoptingTheSameBeamEndWithOneOwnerAndTheLoserRefusesNamingTheWinner", func(t *testing.T) {
		t.Parallel()
		f := newOwnerFixture(t, "neve")
		req := adopt(here, "beamer")
		req.BeforePush = func() {
			if _, err := WriteAdopt(f.otherDir, adopt(there, "beamer")); err != nil {
				t.Errorf("second clone WriteAdopt() error = %v; want nil", err)
			}
		}

		_, err := WriteAdopt(f.boardDir, req)
		refusal := requireHoldRefusal(t, err)
		if refusal.Machine != "osl46" {
			t.Errorf("WriteAdopt() error = %v; want it to name the winner osl46", refusal)
		}
		wantOriginHolder(t, f, "osl46")
		if owner := f.originOwners(t).Entries["beamer"].Owner; owner != "osl46" {
			t.Errorf("origin owner of beamer = %q; want the winner osl46", owner)
		}
	})

	t.Run("ARejectedPushMeetingNoNewHoldReplaysAndLands", func(t *testing.T) {
		t.Parallel()
		f := newOwnerFixture(t, "neve")
		req := adopt(here, "beamer")
		req.BeforePush = func() {
			f.pushFromOther(t, func(store *Store) {
				for i := range store.tasks {
					if store.tasks[i].Slug == "beta" {
						store.tasks[i].Brief = "changed elsewhere"
					}
				}
			})
		}

		got := mustAdopt(t, f, req)
		if got.Hold.Machine != "neve" || !slices.Equal(got.Recorded, []string{"beamer"}) {
			t.Errorf("WriteAdopt() = %+v; want neve's hold and beamer recorded", got)
		}
		origin := f.originOwners(t)
		if origin.Entries["beamer"].Owner != "neve" || len(origin.Holds) != 1 {
			t.Errorf("origin beamer owner %q, holds %+v; want neve and the one hold", origin.Entries["beamer"].Owner, origin.Holds)
		}
		if brief := gitkit.Git(t, f.bare, "show", "main:board.json"); !strings.Contains(brief, "changed elsewhere") {
			t.Errorf("origin board.json lost the other clone's change")
		}
	})

	t.Run("ARefusedRegistrationWritesNothing", func(t *testing.T) {
		t.Parallel()
		f := newOwnerFixture(t, "neve")
		if _, err := RegisterMachine(f.otherDir, Registration{Identity: machinename.Identity{ID: "id-b", Name: "neve"}}); err != nil {
			t.Fatalf("RegisterMachine() from the second clone error = %v; want nil", err)
		}
		head := gitkit.RevParse(t, f.bare, "main")

		_, err := WriteAdopt(f.boardDir, adopt(here, "beamer"))
		var refusal *RegistrationError
		if !errors.As(err, &refusal) {
			t.Fatalf("WriteAdopt() error = %v; want a *RegistrationError", err)
		}
		if after := gitkit.RevParse(t, f.bare, "main"); after != head {
			t.Errorf("origin head = %s after a refusal; want it to stay %s", after, head)
		}
		if holds := f.originOwners(t).Holds; len(holds) != 0 {
			t.Errorf("origin holds = %+v; want none", holds)
		}
	})

	t.Run("UnreachableOriginIsErrOwnerOriginUnreachable", func(t *testing.T) {
		t.Parallel()
		f := newOwnerFixture(t, "neve")
		gitkit.Git(t, f.boardDir, "remote", "set-url", "origin", unreachableOrigin)

		if _, err := WriteAdopt(f.boardDir, adopt(here, "beamer")); !errors.Is(err, ErrOwnerOriginUnreachable) {
			t.Errorf("WriteAdopt() error = %v; want it to match ErrOwnerOriginUnreachable", err)
		}
	})

	t.Run("FixHoldMakesTheHoldAdoptFixWithTaskFixMachineAndTriesOnOrigin", func(t *testing.T) {
		t.Parallel()
		f := newOwnerFixture(t, "neve")
		mustAdopt(t, f, adopt(here, "beamer", "boxed"))

		got, err := FixHold(f.boardDir, here, "fix-7", map[string]int{"beamer": 2})
		if err != nil {
			t.Fatalf("FixHold() error = %v; want nil", err)
		}
		wantSubjects := []HoldSubject{{Subject: "beamer", PriorOwner: "neve", Tries: 2}, {Subject: "boxed", PriorOwner: "box"}}
		for name, hold := range map[string]Hold{"returned": got, "origin": f.originOwners(t).Holds[0]} {
			if hold.Kind != HoldKindAdoptFix || hold.FixTask != "fix-7" || hold.FixMachine != "neve" || hold.Machine != "neve" || !slices.Equal(hold.Subjects, wantSubjects) {
				t.Errorf("%s hold = %+v; want neve's adopt-fix hold of fix-7 over %v", name, hold, wantSubjects)
			}
		}
	})

	t.Run("FixHoldWithNoHoldRefuses", func(t *testing.T) {
		t.Parallel()
		f := newOwnerFixture(t, "neve")

		_, err := FixHold(f.boardDir, here, "fix-7", map[string]int{"beamer": 1})
		refusal := requireHoldRefusal(t, err)
		if refusal.Machine != "" || !strings.Contains(refusal.Error(), "lym adopt beamer") {
			t.Errorf("FixHold() error = %v; want no machine and the way forward `lym adopt beamer`", refusal)
		}
		if head := gitkit.RevParse(t, f.bare, "main"); head != f.seedHead {
			t.Errorf("origin head = %s; want the seed %s", head, f.seedHead)
		}
	})

	t.Run("FixHoldOverAnotherMachinesHoldRefusesNamingIt", func(t *testing.T) {
		t.Parallel()
		f := newOwnerFixture(t, "neve")
		seed(t, f, thereHold("beamer"))

		_, err := FixHold(f.boardDir, here, "fix-7", map[string]int{"beamer": 1})
		if refusal := requireHoldRefusal(t, err); refusal.Machine != "osl46" {
			t.Errorf("FixHold() error = %v; want it to name osl46", refusal)
		}
		wantOriginHolder(t, f, "osl46")
	})

	t.Run("ClearHoldRemovesTheOwnHold", func(t *testing.T) {
		t.Parallel()
		f := newOwnerFixture(t, "neve")
		mustAdopt(t, f, adopt(here, "beamer"))

		if cleared, err := ClearHold(f.boardDir, here); err != nil || !cleared {
			t.Fatalf("ClearHold() = %v, %v; want true, nil", cleared, err)
		}
		if holds := f.originOwners(t).Holds; len(holds) != 0 {
			t.Errorf("origin holds = %+v; want none", holds)
		}
		if cleared, err := ClearHold(f.boardDir, here); err != nil || cleared {
			t.Errorf("ClearHold() again = %v, %v; want false, nil when no hold stands", cleared, err)
		}
	})

	t.Run("ClearHoldOverAnotherMachinesHoldRefusesNamingItAndLeavesIt", func(t *testing.T) {
		t.Parallel()
		f := newOwnerFixture(t, "neve")
		seed(t, f, thereHold("beamer"))

		cleared, err := ClearHold(f.boardDir, here)
		if refusal := requireHoldRefusal(t, err); refusal.Machine != "osl46" || cleared {
			t.Errorf("ClearHold() = %v, %v; want false and a refusal naming osl46", cleared, refusal)
		}
		wantOriginHolder(t, f, "osl46")
	})

	// The rows below plant a write through holdSetterBeforePushHook, which is process-global, so they run serially.
	t.Run("FixHoldRejectedOverATakeHoldRefusesNamingTheOtherMachine", func(t *testing.T) {
		f := newOwnerFixture(t, "neve")
		mustAdopt(t, f, adopt(here, "beamer"))
		planted(t, func() {
			if _, err := WriteAdopt(f.otherDir, takeHold(there, "beamer")); err != nil {
				t.Errorf("second clone take-hold WriteAdopt() error = %v; want nil", err)
			}
		})

		_, err := FixHold(f.boardDir, here, "fix-7", map[string]int{"beamer": 1})
		if refusal := requireHoldRefusal(t, err); refusal.Machine != "osl46" {
			t.Errorf("FixHold() error = %v; want it to name osl46", refusal)
		}
		wantOriginHolder(t, f, "osl46")
	})

	t.Run("ClearHoldRejectedOverATakeHoldRefusesNamingTheOtherMachine", func(t *testing.T) {
		f := newOwnerFixture(t, "neve")
		mustAdopt(t, f, adopt(here, "beamer"))
		planted(t, func() {
			if _, err := WriteAdopt(f.otherDir, takeHold(there, "beamer")); err != nil {
				t.Errorf("second clone take-hold WriteAdopt() error = %v; want nil", err)
			}
		})

		cleared, err := ClearHold(f.boardDir, here)
		if refusal := requireHoldRefusal(t, err); refusal.Machine != "osl46" || cleared {
			t.Errorf("ClearHold() = %v, %v; want false and a refusal naming osl46", cleared, refusal)
		}
		wantOriginHolder(t, f, "osl46")
	})

	t.Run("FixHoldWhoseReplayLostItsHoldRefusesInsteadOfReportingSuccess", func(t *testing.T) {
		f := newOwnerFixture(t, "neve")
		mustAdopt(t, f, adopt(here, "beamer", "boxed"))
		planted(t, func() {
			if _, err := ClearHold(f.otherDir, here); err != nil {
				t.Errorf("second clone ClearHold() error = %v; want nil", err)
			}
		})

		_, err := FixHold(f.boardDir, here, "fix-7", map[string]int{"boxed": 1})
		refusal := requireHoldRefusal(t, err)
		if refusal.Machine != "" || refusal.OwnStands {
			t.Errorf("FixHold() error = %v; want a refusal naming no machine with no own hold standing", refusal)
		}
		if !strings.Contains(refusal.Error(), "lym adopt beamer boxed") {
			t.Errorf("FixHold() error = %v; want the way forward to name both subjects of the hold", refusal)
		}
		if holds := f.originOwners(t).Holds; len(holds) != 0 {
			t.Errorf("origin holds = %+v; want none", holds)
		}
	})

	t.Run("ClearHoldWhoseReplayLostItsRemovalRefuses", func(t *testing.T) {
		f := newOwnerFixture(t, "neve")
		mustAdopt(t, f, adopt(here, "beamer"))
		planted(t, func() {
			if _, err := FixHold(f.otherDir, here, "fix-9", map[string]int{"beamer": 5}); err != nil {
				t.Errorf("second clone FixHold() error = %v; want nil", err)
			}
		})

		cleared, err := ClearHold(f.boardDir, here)
		if refusal := requireHoldRefusal(t, err); refusal.Machine != "" || !refusal.OwnStands || cleared {
			t.Errorf("ClearHold() = %v, %v; want false and a refusal naming no machine with this machine's hold standing", cleared, refusal)
		}
		wantOriginHolder(t, f, "neve")
	})
}

func TestWriteHold(t *testing.T) {
	tiergate.Require(t, 1)
	// Not parallel at the top: registering the replayer is process-global, as in TestSetOwners.
	fabricengine.RegisterBoltReplayer(FabricReplay{})

	at := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	hold := Hold{
		Kind:     HoldKindAdopt,
		Machine:  "osl46",
		At:       at,
		Subjects: []HoldSubject{{Subject: "boxed", PriorOwner: "box"}},
	}

	t.Run("AWrittenHoldIsOnOriginWhenItReturnsAndARepeatWritesNothing", func(t *testing.T) {
		t.Parallel()
		f := newOwnerFixture(t, "neve")

		got, err := WriteHold(f.boardDir, hold)
		if err != nil || !got.Wrote || got.Standing != nil {
			t.Fatalf("WriteHold() = %+v, %v; want Wrote with no Standing", got, err)
		}
		holds := f.originOwners(t).Holds
		if len(holds) != 1 || holds[0].Kind != hold.Kind || holds[0].Machine != hold.Machine ||
			!reflect.DeepEqual(holds[0].Subjects, hold.Subjects) || !holds[0].At.Equal(at) {
			t.Errorf("origin holds = %+v; want the written hold %+v", holds, hold)
		}

		head := gitkit.RevParse(t, f.bare, "main")
		got, err = WriteHold(f.boardDir, hold)
		if err != nil || got.Wrote || got.Standing != nil {
			t.Fatalf("WriteHold() again = %+v, %v; want nothing written and no Standing", got, err)
		}
		if after := gitkit.RevParse(t, f.bare, "main"); after != head {
			t.Errorf("origin head = %s after a repeat; want it to stay %s", after, head)
		}
	})

	t.Run("AHoldAnotherCloneWroteFirstIsReturnedAsStandingAndNothingIsWritten", func(t *testing.T) {
		t.Parallel()
		f := newOwnerFixture(t, "neve")
		f.pushFromOther(t, func(store *Store) {
			store.holds = append(store.holds, Hold{Kind: HoldKindAdopt, Machine: "box", At: at, Subjects: []HoldSubject{{Subject: "root"}}})
		})
		head := gitkit.RevParse(t, f.bare, "main")

		got, err := WriteHold(f.boardDir, hold)
		if err != nil || got.Wrote || got.Standing == nil || got.Standing.Machine != "box" {
			t.Fatalf("WriteHold() = %+v, %v; want nothing written and box's hold standing", got, err)
		}
		if after := gitkit.RevParse(t, f.bare, "main"); after != head {
			t.Errorf("origin head = %s after a lost write; want it to stay %s", after, head)
		}
	})

	t.Run("AHoldUnderAFormerNameOfTheWriterStandsAsItsOwnAndNothingIsWritten", func(t *testing.T) {
		t.Parallel()
		f := newOwnerFixture(t, "neve")
		formerHold := hold
		formerHold.Machine = "neve"
		if got, err := WriteHold(f.boardDir, formerHold); err != nil || !got.Wrote {
			t.Fatalf("WriteHold(neve) = %+v, %v; want Wrote", got, err)
		}
		for _, identity := range []machinename.Identity{
			{ID: "id-a", Name: "neve"},
			{ID: "id-a", Name: "osl46", Former: []string{"neve"}},
		} {
			if _, err := RegisterMachine(f.boardDir, Registration{Identity: identity}); err != nil {
				t.Fatalf("RegisterMachine(%s) error = %v; want nil", identity.Name, err)
			}
		}
		head := gitkit.RevParse(t, f.bare, "main")

		got, err := WriteHold(f.boardDir, hold)
		if err != nil || got.Wrote || got.Standing != nil {
			t.Fatalf("WriteHold() = %+v, %v; want nothing written and no Standing", got, err)
		}
		if after := gitkit.RevParse(t, f.bare, "main"); after != head {
			t.Errorf("origin head = %s after a repeat under the former name; want it to stay %s", after, head)
		}
		if holds := f.originOwners(t).Holds; len(holds) != 1 {
			t.Errorf("origin holds = %+v; want the one hold under the former name", holds)
		}
	})

	t.Run("AHoldOfAnotherKindFromTheSameMachineIsAppendedBesideTheFirst", func(t *testing.T) {
		t.Parallel()
		f := newOwnerFixture(t, "neve")
		fixHold := hold
		fixHold.Kind, fixHold.FixTask = HoldKindAdoptFix, "fix-1"

		for _, written := range []Hold{hold, fixHold} {
			if got, err := WriteHold(f.boardDir, written); err != nil || !got.Wrote || got.Standing != nil {
				t.Fatalf("WriteHold(%s) = %+v, %v; want Wrote with no Standing", written.Kind, got, err)
			}
		}

		holds := f.originOwners(t).Holds
		if len(holds) != 2 || holds[0].Kind != HoldKindAdopt || holds[1].Kind != HoldKindAdoptFix {
			t.Errorf("origin holds = %+v; want the adopt hold then the adopt-fix hold", holds)
		}
	})

	t.Run("AHoldOfAnUnknownKindOrAnAdoptFixHoldWithNoFixTaskIsRefusedNamingTheField", func(t *testing.T) {
		t.Parallel()
		f := newOwnerFixture(t, "neve")
		for name, bad := range map[string]Hold{
			"Kind":    {Kind: "pause", Machine: "osl46"},
			"FixTask": {Kind: HoldKindAdoptFix, Machine: "osl46"},
		} {
			if got, err := WriteHold(f.boardDir, bad); err == nil || got.Wrote {
				t.Errorf("WriteHold(%s) = %+v, %v; want a refusal", name, got, err)
			}
		}
		if head := gitkit.RevParse(t, f.bare, "main"); head != f.seedHead {
			t.Errorf("origin head = %s after refusals; want the seed %s", head, f.seedHead)
		}
	})
}

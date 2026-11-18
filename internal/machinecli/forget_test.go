// forget_test.go covers `lym machine name --forget-former <name>` over fake hubs, and the references it lists.

package machinecli

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestName_ForgetFormer(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const content = `{"name":"osl46","names":["neve","osl46"],"id":"id1"}`

	t.Run("TheNameLeavesNamesJoinsForgottenKeepsTheIdAndLeavesTheHubEntry", func(t *testing.T) {
		t.Parallel()
		f := newFakeHubs(hubA)
		path := machineFileAt(t, content, false)
		before := boardengine.Registry{"id2": {Name: "neve"}}
		registration := boardengine.Registration{Identity: machinename.Identity{ID: "id1", Name: "osl46", Former: []string{"neve"}}}
		if err := before.Check(registration); err == nil {
			t.Fatal("Check() before the forget = nil; want the former name refused as another machine's current name")
		}

		code, envelope := runName(t, seams{path: path, host: "box"}.deps(f), t.TempDir(), "--forget-former", "neve")

		if code != 0 || envelope["ok"] != true {
			t.Fatalf("forget exit %d, envelope %v; want ok", code, envelope)
		}
		var file machinename.File
		if err := json.Unmarshal([]byte(strings.TrimPrefix(fileState(t, path), "file:")), &file); err != nil {
			t.Fatalf("decode the written file: %v", err)
		}
		if file.Name != "osl46" || !slices.Equal(file.Names, []string{"osl46"}) || file.ID != "id1" || !slices.Equal(file.Forgotten, []string{"neve"}) {
			t.Errorf("written file = %+v; want name osl46, names [osl46], id id1, forgotten [neve]", file)
		}
		registrations := f.registrationsFor(hubA)
		if len(registrations) != 1 || !slices.Equal(registrations[0].Forget, []string{"neve"}) || registrations[0].Identity.ID != "id1" || len(registrations[0].Identity.Former) != 0 {
			t.Fatalf("registrations = %+v; want one with id id1, no former names and neve forgotten", registrations)
		}
		if err := before.Check(registrations[0]); err != nil {
			t.Errorf("Check() after the forget = %v; want the registration to pass", err)
		}
		if requests := f.requestTotal(); requests != 0 {
			t.Errorf("owner writes = %d; want none, since the forget writes no owner", requests)
		}
	})

	t.Run("ANameTheFileDoesNotHoldIsRefusedNamingItsFormerNamesAndWritesNothing", func(t *testing.T) {
		t.Parallel()
		f := newFakeHubs(hubA)
		path := machineFileAt(t, content, false)

		code, envelope := runName(t, seams{path: path, host: "box"}.deps(f), t.TempDir(), "--forget-former", "other")

		message, _ := envelope["error"].(string)
		if code == 0 || envelope["ok"] != false || !strings.Contains(message, `"other"`) || !strings.Contains(message, "[neve]") {
			t.Errorf("forget exit %d, envelope %v; want a refusal naming the file's former names", code, envelope)
		}
		if fileState(t, path) != "file:"+content || f.registrationTotal() != 0 {
			t.Errorf("file = %q, registrations = %d; want both untouched", fileState(t, path), f.registrationTotal())
		}
	})

	t.Run("APositionalNameWithTheFlagIsRefused", func(t *testing.T) {
		t.Parallel()
		f := newFakeHubs(hubA)
		path := machineFileAt(t, content, false)

		code, envelope := runName(t, seams{path: path, host: "box"}.deps(f), t.TempDir(), "osl47", "--forget-former", "neve")

		if code == 0 || envelope["ok"] != false || fileState(t, path) != "file:"+content || f.registrationTotal() != 0 {
			t.Errorf("exit %d, envelope %v, file %q; want a refusal that writes nothing", code, envelope, fileState(t, path))
		}
	})

	t.Run("AFailedHubIsReportedWithTheRerunAndTheOthersGoOn", func(t *testing.T) {
		t.Parallel()
		f := newFakeHubs(hubA, hubB)
		f.registerErr[f.board(hubA)] = errors.New("push boom")
		f.registered[f.board(hubB)] = true
		path := machineFileAt(t, content, false)
		d := seams{path: path, host: "box"}.deps(f)

		code, envelope := runName(t, d, t.TempDir(), "--forget-former", "neve")

		if code == 0 || envelope["ok"] != false || !strings.Contains(envelope["error"].(string), "lym machine name --forget-former neve") {
			t.Fatalf("forget exit %d, envelope %v; want a non-zero exit naming the re-run", code, envelope)
		}
		outcomes := hubOutcomes(t, envelope)
		if outcomes[0]["outcome"] != outcomeFailed || !strings.Contains(outcomes[0]["way_forward"].(string), "lym machine name --forget-former neve") {
			t.Errorf("failed hub outcome = %v; want the registration failure and the re-run", outcomes[0])
		}
		if outcomes[1]["outcome"] != outcomeRegistered || outcomes[1]["registered"] != true {
			t.Errorf("second hub outcome = %v; want registered despite the first hub's failure", outcomes[1])
		}

		delete(f.registerErr, f.board(hubA))
		afterFirst := fileState(t, path)
		code, envelope = runName(t, d, t.TempDir(), "--forget-former", "neve")

		if code != 0 || envelope["ok"] != true {
			t.Fatalf("re-run exit %d, envelope %v; want ok, the forgotten name passing as the re-run", code, envelope)
		}
		if after := fileState(t, path); after != afterFirst {
			t.Errorf("machine file after the re-run = %q; want it unchanged from %q", after, afterFirst)
		}
	})

	t.Run("ReferencesAreListedAndAHubWhoseReferencesDoNotReadIsReported", func(t *testing.T) {
		t.Parallel()
		f := newFakeHubs(hubA, hubB)
		f.references[hubA] = []formerReference{{Hub: hubA, Kind: referenceOwner, Entry: "b1", Subject: "b1", WayForward: "run `lym adopt b1`"}}
		f.referencesErr[hubB] = errors.New("board unreadable")
		path := machineFileAt(t, content, false)

		code, envelope := runName(t, seams{path: path, host: "box"}.deps(f), t.TempDir(), "--forget-former", "neve")

		if code != 0 {
			t.Fatalf("forget exit %d, envelope %v; want ok", code, envelope)
		}
		machine := machineOf(t, envelope)
		references, _ := machine["references"].([]any)
		if len(references) != 1 || references[0].(map[string]any)["subject"] != "b1" {
			t.Errorf("machine.references = %v; want the hub a reference with subject b1", machine["references"])
		}
		if errs, _ := machine["reference_errors"].([]any); len(errs) != 1 || !strings.Contains(errs[0].(string), "board unreadable") {
			t.Errorf("machine.reference_errors = %v; want the hub b read failure", machine["reference_errors"])
		}
	})

	t.Run("CollectReferencesListsOwnersTakeoversAndCancelsWithTheirAdoptSubjects", func(t *testing.T) {
		t.Parallel()
		owners := boardengine.Owners{
			Prime: "neve",
			Entries: map[string]boardengine.OwnerEntry{
				"beam":     {IsProject: true, Beam: true, Owner: "neve"},
				"other":    {IsProject: true, Beam: true, Owner: "elsewhere"},
				"member":   {IsTask: true},
				"loose":    {IsTask: true},
				"mover":    {IsTask: true},
				"outsider": {IsTask: true},
			},
		}
		parentOf := func(slug string) (string, error) {
			if slug == "loose" {
				return "", nil
			}
			return "beam", nil
		}
		runs := map[string]runClaims{
			"member":   {takeover: "neve"},
			"loose":    {cancel: "neve"},
			"mover":    {takeover: "neve", cancel: "neve"},
			"outsider": {takeover: "elsewhere", cancel: "elsewhere"},
		}

		got, err := collectReferences(hubA, "neve", owners, parentOf, runs)

		if err != nil {
			t.Fatalf("collectReferences() error = %v; want nil", err)
		}
		type reference struct{ kind, entry, subject string }
		var gotRefs []reference
		for _, found := range got {
			gotRefs = append(gotRefs, reference{found.Kind, found.Entry, found.Subject})
			if !strings.Contains(found.WayForward, "lym adopt "+found.Subject) {
				t.Errorf("reference %+v way forward = %q; want it to name lym adopt %s", found, found.WayForward, found.Subject)
			}
		}
		want := []reference{
			{referenceOwner, boardengine.OwnerWritePrime, boardengine.RootSubject},
			{referenceOwner, "beam", "beam"},
			{referenceCancel, "loose", boardengine.RootSubject},
			{referenceTakeover, "member", "beam"},
			{referenceTakeover, "mover", "beam"},
			{referenceCancel, "mover", "beam"},
		}
		if !slices.Equal(gotRefs, want) {
			t.Errorf("collectReferences() = %v; want %v", gotRefs, want)
		}
	})
}

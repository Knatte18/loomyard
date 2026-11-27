// latercaller_test.go covers create-caller-later over plans parsed from card files.

package planparser_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestValidateFormat_CreateCallerLater covers create-caller-later:
// a card creating a production function it cannot call, which a later card that can call it names.
// Every finding sits on card 1 and carries the row's wantRef.
func TestValidateFormat_CreateCallerLater(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const (
		level        = "plan:internal/watch#Level"
		levelDecl    = level + "` -> `func Level(name string) string"
		method       = "plan:internal/watch#Watcher.Level"
		methodDecl   = method + "` -> `func (w *Watcher) Level() string"
		levelFinding = `card 1 creates "plan:internal/watch#Level", but no target of card 1 can call it from production code, and card 2 names it; ` +
			`a function no production code calls fails card 1's gate on the guard that every production function is reachable from a main, ` +
			`so move the Create to card 2, the card that adds its first production caller, or list that caller on card 1`
	)
	creator := func(groups ...plankit.Group) plankit.Card {
		return plankit.Card{Number: 1, Slug: "create", Groups: groups, ImpactSummary: "one package"}
	}
	caller := func(uses []string, groups ...plankit.Group) plankit.Card {
		return plankit.Card{Number: 2, Slug: "wire", Groups: groups, Uses: uses, ImpactSummary: "one package"}
	}
	create := func(targets ...string) plankit.Group { return plankit.Group{Label: "Create", Targets: targets} }
	edit := func(targets ...string) plankit.Group { return plankit.Group{Label: "Edit", Targets: targets} }
	extends := func(targets ...string) plankit.Group { return plankit.Group{Label: "Extends", Targets: targets} }
	custom := func(targets ...string) plankit.Group { return plankit.Group{Label: "Custom", Targets: targets} }
	remove := func(targets ...string) plankit.Group { return plankit.Group{Label: "Delete", Targets: targets} }
	// wired is the plan of a creating card and a later card that uses level and edits a production member.
	wired := func(groups ...plankit.Group) []plankit.Card {
		return []plankit.Card{creator(groups...), caller([]string{level}, edit("internal/dobby#Tick"))}
	}

	tests := []struct {
		name     string
		language string
		cards    []plankit.Card
		want     []string
		wantRef  string
	}{
		{
			name: "a function whose only caller is on a later card",
			cards: []plankit.Card{
				creator(create(levelDecl, "internal/watch/level_test.go"), edit("internal/watch/template.yaml")),
				caller([]string{level}, edit("internal/dobby#Tick")),
			},
			want:    []string{levelFinding},
			wantRef: level,
		},
		{
			name: "a later card that names the function among its targets",
			cards: []plankit.Card{
				creator(create(levelDecl)),
				caller(nil, edit(level, "internal/dobby/tick.go")),
			},
			want:    []string{levelFinding},
			wantRef: level,
		},
		{
			name: "a method whose only caller is on a later card",
			cards: []plankit.Card{
				creator(create("plan:internal/watch#Watcher` -> `type Watcher struct", methodDecl)),
				caller([]string{method}, edit("internal/dobby#")),
			},
			want: []string{
				`card 1 creates "plan:internal/watch#Watcher.Level", but no target of card 1 can call it from production code, and card 2 names it; ` +
					`a function no production code calls fails card 1's gate on the guard that every production function is reachable from a main, ` +
					`so move the Create to card 2, the card that adds its first production caller, or list that caller on card 1`,
			},
			wantRef: method,
		},
		{
			name:  "the creating card extends a production member that can call it",
			cards: wired(create(levelDecl), extends("internal/dobby#Tick")),
		},
		{
			name:  "the creating card has a custom change to a production file",
			cards: wired(create(levelDecl), custom("internal/dobby/tick.go")),
		},
		{
			name:    "the creating card otherwise only deletes",
			cards:   wired(create(levelDecl), remove("internal/dobby#Old")),
			want:    []string{levelFinding},
			wantRef: level,
		},
		{
			name:    "the creating card creates a second function",
			cards:   wired(create(levelDecl, "plan:internal/watch#format` -> `func format() string")),
			want:    []string{levelFinding},
			wantRef: level,
		},
		{
			name:    "the creating card creates a constant",
			cards:   wired(create(levelDecl, "plan:internal/watch#Limit` -> `const Limit = 3")),
			want:    []string{levelFinding},
			wantRef: level,
		},
		{
			name:    "the creating card edits a test handle it does not create",
			cards:   wired(create(levelDecl), edit("plan:internal/watch_test#helper")),
			want:    []string{levelFinding},
			wantRef: level,
		},
		{
			name:  "the creating card edits a production handle it does not create",
			cards: wired(create(levelDecl), edit("plan:internal/other#Thing")),
		},
		{
			name:  "the creating card creates a function of a test-support directory",
			cards: wired(create(levelDecl, "plan:internal/testkit/watchkit#Probe` -> `func Probe() string")),
		},
		{
			name: "a later card whose refs are a test by name in a production package",
			cards: []plankit.Card{
				creator(create(levelDecl)),
				caller([]string{level}, edit("internal/dobby#TestTick", "plan:internal/watch#TestHelper")),
			},
		},
		{
			name: "two later cards that can call the function",
			cards: []plankit.Card{
				creator(create(levelDecl)),
				caller([]string{level}, edit("internal/dobby#Tick")),
				{Number: 3, Slug: "report", Groups: []plankit.Group{edit("internal/dobby#Report")}, Uses: []string{level}, ImpactSummary: "one package"},
			},
			want:    []string{levelFinding},
			wantRef: level,
		},
		{
			name: "a later card whose production ref is a bare symbol",
			cards: []plankit.Card{
				creator(create(levelDecl)),
				caller([]string{level}, edit("dobby.Tick")),
			},
			want:    []string{levelFinding},
			wantRef: level,
		},
		{
			name: "a later card that creates the caller as a new function",
			cards: []plankit.Card{
				creator(create(levelDecl)),
				caller([]string{level}, create("plan:internal/watch#Run` -> `func Run()")),
			},
			want:    []string{levelFinding},
			wantRef: level,
		},
		{
			name: "a later card that creates only a test helper taking a testing type",
			cards: []plankit.Card{
				creator(create(levelDecl)),
				caller([]string{level}, create("plan:internal/watch#newFixture` -> `func newFixture(t testing.TB)")),
			},
		},
		{
			name: "a later card whose refs are an external test package and its member",
			cards: []plankit.Card{
				creator(create(levelDecl)),
				caller([]string{level}, edit("internal/dobby_test#", "internal/watch_test#helper")),
			},
		},
		{
			name: "a function of a test-support directory",
			cards: []plankit.Card{
				creator(create("plan:internal/gitkit#NewRepo` -> `func NewRepo() string")),
				caller([]string{"plan:internal/gitkit#NewRepo"}, edit("internal/dobby#Tick")),
			},
		},
		{
			name: "a function below a test-support directory",
			cards: []plankit.Card{
				creator(create("plan:internal/testkit/watchkit#New` -> `func New() string")),
				caller([]string{"plan:internal/testkit/watchkit#New"}, edit("internal/dobby#Tick")),
			},
		},
		{
			name: "a function named as a test without a testing type",
			cards: []plankit.Card{
				creator(create("plan:internal/watch#ExampleLevel` -> `func ExampleLevel()")),
				caller([]string{"plan:internal/watch#ExampleLevel"}, edit("internal/dobby#Tick")),
			},
		},
		{
			name: "the creating card edits a production member that can call it",
			cards: []plankit.Card{
				creator(create(levelDecl), edit("internal/dobby#Tick")),
				caller([]string{level}, edit("internal/dobby#Report")),
			},
		},
		{
			name: "the creating card creates a production file that can call it",
			cards: []plankit.Card{
				creator(create(levelDecl, "internal/watch/wire.go")),
				caller([]string{level}, edit("internal/dobby#Tick")),
			},
		},
		{
			name: "the creating card creates a variable that can call it",
			cards: []plankit.Card{
				creator(create(levelDecl, "plan:internal/watch#levels` -> `var levels = map[string]func(string) string")),
				caller([]string{level}, edit("internal/dobby#Tick")),
			},
		},
		{
			name: "the later card only changes the function and tests",
			cards: []plankit.Card{
				creator(create(levelDecl)),
				caller(nil, edit(level, "internal/watch_test#TestLevel", "internal/watch/level_test.go")),
			},
		},
		{
			name: "no later card names the function",
			cards: []plankit.Card{
				creator(create(levelDecl)),
				caller(nil, edit("internal/dobby#Tick")),
			},
		},
		{
			name: "a test helper taking a testing type",
			cards: []plankit.Card{
				creator(create("plan:internal/watch#newFixture` -> `func newFixture(t testing.TB) string")),
				caller([]string{"plan:internal/watch#newFixture"}, edit("internal/dobby#Tick")),
			},
		},
		{
			name: "a function of an external test package",
			cards: []plankit.Card{
				creator(create("plan:internal/watch_test#helper` -> `func helper() string")),
				caller([]string{"plan:internal/watch_test#helper"}, edit("internal/dobby#Tick")),
			},
		},
		{
			name: "a program entry point",
			cards: []plankit.Card{
				creator(create("plan:cmd/watch#main` -> `func main()")),
				caller([]string{"plan:cmd/watch#main"}, edit("internal/dobby#Tick")),
			},
		},
		{
			name: "a type, which holds no code to reach",
			cards: []plankit.Card{
				creator(create("plan:internal/watch#Watcher` -> `type Watcher struct")),
				caller([]string{"plan:internal/watch#Watcher"}, edit("internal/dobby#Tick")),
			},
		},
		{
			name:     "a plan with no glyph language",
			language: "none",
			cards: []plankit.Card{
				creator(create(levelDecl)),
				caller([]string{level}, edit("internal/dobby/tick.go")),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "plan")
			plankit.Write(t, dir, plankit.Plan{Language: tt.language, Cards: tt.cards})
			plan, err := planparser.ParsePlan(dir)
			if err != nil {
				t.Fatalf("ParsePlan() failed: %v", err)
			}

			var got []string
			for _, finding := range planparser.ValidateFormat(plan, t.TempDir()) {
				if finding.Check != "create-caller-later" {
					continue
				}
				if finding.Card != "1-create" {
					t.Errorf("create-caller-later finding attributed to %q; want %q", finding.Card, "1-create")
				}
				if finding.Ref != tt.wantRef {
					t.Errorf("create-caller-later finding Ref = %q; want %q", finding.Ref, tt.wantRef)
				}
				got = append(got, finding.Detail)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("ValidateFormat() create-caller-later details = %q; want %q", got, tt.want)
			}
		})
	}
}

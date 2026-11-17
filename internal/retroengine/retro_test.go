// retro_test.go covers how a retro finds the last retro, chooses its runs and renders its task body.

package retroengine_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/retroengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// at returns the UTC time of day on 2026-11-16 for hour, minute and second.
func at(hour, minute, second int) time.Time {
	return time.Date(2026, 11, 16, hour, minute, second, 0, time.UTC)
}

func TestLastRetro(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name      string
		names     []string
		want      time.Time
		wantFound bool
	}{
		{"no names", nil, time.Time{}, false},
		{"no retro form among the names", []string{"lym-retro", "retro-notes", "retro-2026-11-16"}, time.Time{}, false},
		{"the newest stamp wins over older and foreign names", []string{"retro-20261110-090000", "board-task", "retro-20261116-140550", "retro-20261112-235959"}, at(14, 5, 50), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, found := retroengine.LastRetro(tt.names)
			if !got.Equal(tt.want) || found != tt.wantFound {
				t.Errorf("LastRetro(%v) = %v, %v; want %v, %v", tt.names, got, found, tt.want, tt.wantFound)
			}
		})
	}
}

func TestSelect(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	landed := []retroengine.Run{
		{Slug: "old", Landed: at(8, 0, 0)},
		{Slug: "a", Landed: at(10, 0, 0)},
		{Slug: "b", Landed: at(11, 0, 0)},
		{Slug: "relanded", Landed: at(9, 0, 0)},
		{Slug: "relanded", Landed: at(12, 0, 0)},
		{Slug: "late", Landed: at(15, 0, 0)},
	}
	now := at(14, 0, 0)
	tests := []struct {
		name string
		// landed replaces the shared landings when set.
		landed   []retroengine.Run
		last     time.Time
		hasLast  bool
		window   retroengine.Window
		want     retroengine.Plan
		wantErrs []string
	}{
		{
			name:    "after the last retro and up to now, each run at its newest landing",
			last:    at(9, 30, 0),
			hasLast: true,
			window:  retroengine.Window{Now: now, MaxRuns: 10},
			want: retroengine.Plan{Slug: "retro-20261116-140000", From: at(9, 30, 0), Until: now,
				Runs: []retroengine.Run{{Slug: "a", Landed: at(10, 0, 0)}, {Slug: "b", Landed: at(11, 0, 0)}, {Slug: "relanded", Landed: at(12, 0, 0)}}},
		},
		{
			name:   "with no earlier retro, the default window before now",
			window: retroengine.Window{Now: at(14, 0, 0).Add(retroengine.DefaultWindow - 5*time.Hour), MaxRuns: 10},
			want: retroengine.Plan{Slug: "retro-20261123-090000", From: at(9, 0, 0), Until: at(14, 0, 0).Add(retroengine.DefaultWindow - 5*time.Hour),
				Runs: []retroengine.Run{{Slug: "a", Landed: at(10, 0, 0)}, {Slug: "b", Landed: at(11, 0, 0)}, {Slug: "relanded", Landed: at(12, 0, 0)}, {Slug: "late", Landed: at(15, 0, 0)}}},
		},
		{
			name:    "a given since and until replace the last retro and now",
			last:    at(11, 30, 0),
			hasLast: true,
			window:  retroengine.Window{Since: at(7, 0, 0), Until: at(10, 0, 0), Now: now, MaxRuns: 10},
			want: retroengine.Plan{Slug: "retro-20261116-100000", From: at(7, 0, 0), Until: at(10, 0, 0),
				Runs: []retroengine.Run{{Slug: "old", Landed: at(8, 0, 0)}, {Slug: "a", Landed: at(10, 0, 0)}}},
		},
		{
			name:    "past the cap the oldest runs are kept and the window ends at the last kept landing",
			last:    at(7, 0, 0),
			hasLast: true,
			window:  retroengine.Window{Now: now, MaxRuns: 2},
			want: retroengine.Plan{Slug: "retro-20261116-100000", From: at(7, 0, 0), Until: at(10, 0, 0),
				Runs: []retroengine.Run{{Slug: "old", Landed: at(8, 0, 0)}, {Slug: "a", Landed: at(10, 0, 0)}}},
		},
		{
			name: "a cap between runs landed in one second leaves that second to the next retro",
			landed: []retroengine.Run{
				{Slug: "x", Landed: at(8, 0, 0)}, {Slug: "y", Landed: at(10, 0, 0)}, {Slug: "z", Landed: at(10, 0, 0)}, {Slug: "w", Landed: at(11, 0, 0)},
			},
			last:    at(7, 0, 0),
			hasLast: true,
			window:  retroengine.Window{Now: now, MaxRuns: 2},
			want: retroengine.Plan{Slug: "retro-20261116-080000", From: at(7, 0, 0), Until: at(8, 0, 0),
				Runs: []retroengine.Run{{Slug: "x", Landed: at(8, 0, 0)}}},
		},
		{
			name:    "a cap inside the oldest landing's second keeps that whole second",
			landed:  []retroengine.Run{{Slug: "a", Landed: at(10, 0, 0)}, {Slug: "b", Landed: at(10, 0, 0)}, {Slug: "c", Landed: at(10, 0, 0)}, {Slug: "d", Landed: at(11, 0, 0)}},
			last:    at(7, 0, 0),
			hasLast: true,
			window:  retroengine.Window{Now: now, MaxRuns: 2},
			want: retroengine.Plan{Slug: "retro-20261116-100000", From: at(7, 0, 0), Until: at(10, 0, 0),
				Runs: []retroengine.Run{{Slug: "a", Landed: at(10, 0, 0)}, {Slug: "b", Landed: at(10, 0, 0)}, {Slug: "c", Landed: at(10, 0, 0)}}},
		},
		{
			name:    "a cap inside a second that holds every run keeps them all",
			landed:  []retroengine.Run{{Slug: "a", Landed: at(10, 0, 0)}, {Slug: "b", Landed: at(10, 0, 0)}},
			last:    at(7, 0, 0),
			hasLast: true,
			window:  retroengine.Window{Now: now, MaxRuns: 1},
			want: retroengine.Plan{Slug: "retro-20261116-100000", From: at(7, 0, 0), Until: at(10, 0, 0),
				Runs: []retroengine.Run{{Slug: "a", Landed: at(10, 0, 0)}, {Slug: "b", Landed: at(10, 0, 0)}}},
		},
		{
			name:    "named runs are covered exactly, whatever the last retro, from just before the oldest landing",
			last:    at(13, 0, 0),
			hasLast: true,
			window:  retroengine.Window{Slugs: []string{"late", "old", "late"}, Now: now, MaxRuns: 2},
			want: retroengine.Plan{Slug: "retro-20261116-150000", From: at(7, 59, 59), Until: at(15, 0, 0),
				Runs: []retroengine.Run{{Slug: "old", Landed: at(8, 0, 0)}, {Slug: "late", Landed: at(15, 0, 0)}}},
		},
		{
			name:     "a named run with no landing within the lookback is refused with the lookback's start",
			window:   retroengine.Window{Slugs: []string{"a", "never-landed"}, Now: now, MaxRuns: 10},
			wantErrs: []string{`"never-landed"`, "within the last 60 days", "since 2026-09-17T14:00:00Z", "--since and --until"},
		},
		{
			name:     "more named runs than the cap are refused",
			window:   retroengine.Window{Slugs: []string{"a", "b", "old"}, Now: now, MaxRuns: 2},
			wantErrs: []string{"3 runs named", "cap of 2", "--max-runs"},
		},
		{
			name:     "a window with no landed run is refused",
			last:     at(12, 0, 0),
			hasLast:  true,
			window:   retroengine.Window{Now: now, MaxRuns: 10},
			wantErrs: []string{"no run landed after 2026-11-16T12:00:00Z and up to 2026-11-16T14:00:00Z", "--since"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runs := landed
			if tt.landed != nil {
				runs = tt.landed
			}
			got, err := retroengine.Select(runs, tt.last, tt.hasLast, tt.window)
			if len(tt.wantErrs) > 0 {
				if err == nil {
					t.Fatalf("Select() = %+v, nil; want an error", got)
				}
				for _, want := range tt.wantErrs {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("Select() error = %q; want it to contain %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("Select() = _, %v; want nil error", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Select() = %+v; want %+v", got, tt.want)
			}
		})
	}
}

func TestPlan_TaskFieldsFilesOneDarnTaskOnTheDocsProfile(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	plan := retroengine.Plan{Slug: "retro-20261116-140000", From: at(9, 0, 0), Until: at(14, 0, 0),
		Runs: []retroengine.Run{{Slug: "a", Landed: at(10, 0, 0)}, {Slug: "b", Landed: at(11, 0, 0)}}}

	got := plan.TaskFields("the body\n")

	for key, want := range map[string]any{
		"slug":    "retro-20261116-140000",
		"title":   "Retro of 2 landed runs up to 2026-11-16T14:00:00Z",
		"kind":    boardengine.KindTask,
		"recipe":  shedrun.RecipeDarn,
		"profile": "docs",
		"labels":  []string{"research"},
		"body":    "the body\n",
	} {
		if !reflect.DeepEqual(got[key], want) {
			t.Errorf("TaskFields()[%q] = %v; want %v", key, got[key], want)
		}
	}
}

func TestPlan_BodyFillsTheShippedStencil(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	dir := t.TempDir()
	shipped, ok := stencils.Registry().Default(retroengine.StencilName)
	if !ok {
		t.Fatalf("stencil %q is not registered", retroengine.StencilName)
	}
	file := stencilstore.Path(dir, retroengine.StencilName)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, shipped, 0o644); err != nil {
		t.Fatal(err)
	}
	plan := retroengine.Plan{Slug: "retro-20261116-140000", From: at(9, 0, 0), Until: at(14, 0, 0),
		Runs: []retroengine.Run{{Slug: "a", Landed: at(10, 0, 0)}, {Slug: "b", Landed: at(11, 0, 0)}}}

	body, err := plan.Body(dir)
	if err != nil {
		t.Fatalf("Body() = _, %v; want nil error", err)
	}
	for _, want := range []string{
		"- `a`, landed 2026-11-16T10:00:00Z\n- `b`, landed 2026-11-16T11:00:00Z",
		"`tools/retro/retro.sh docs/research/retro/retro-20261116-140000 a b`",
		"--runs a,b ",
		"docs/research/retro/retro-20261116-140000/report.md",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Body() does not contain %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "{{") || strings.Contains(body, "<no value>") {
		t.Errorf("Body() left a marker unfilled:\n%s", body)
	}
}

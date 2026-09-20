package loomcli

import (
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestDriverReportPath_FrozenClock_TwoCallsProduceDifferentPaths is the load-bearing case: two calls
// under a frozen clock must produce two different paths. A test that instead advances the clock
// between calls would prove only that the timestamp varies -- the half that already worked -- and
// would pass against a composer with no random component at all. This is the only case that can
// fail against a composer missing the random half, and the collision it guards fires precisely under
// the automated relaunch batch 7 drives.
func TestDriverReportPath_FrozenClock_TwoCallsProduceDifferentPaths(t *testing.T) {
	l := &lyxcwd.Location{}
	frozen := func() time.Time { return time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC) }

	seq := []string{"aaaa", "bbbb"}
	i := 0
	stubRand := func() string {
		v := seq[i]
		i++
		return v
	}

	got1 := driverReportPath(l, "self", frozen, stubRand)
	got2 := driverReportPath(l, "self", frozen, stubRand)

	if got1 == got2 {
		t.Fatalf("driverReportPath() under a frozen clock produced the same path twice: %q -- a same-second relaunch would collide against Spec.validate's pre-existing-file refusal", got1)
	}
}

// TestDriverReportPath_LandsUnderRunScratchDir asserts the composed path lands under the run's
// ephemeral scratch directory, composed through shedrun.ScratchDir.
func TestDriverReportPath_LandsUnderRunScratchDir(t *testing.T) {
	l := &lyxcwd.Location{}
	now := func() time.Time { return time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC) }
	stubRand := func() string { return "cafe" }

	got := driverReportPath(l, "self", now, stubRand)
	want := shedrun.ScratchDir(l, "self")
	if !strings.HasPrefix(got, want+string([]rune{'/'})) && !strings.HasPrefix(got, want) {
		t.Errorf("driverReportPath() = %q; want it to land under scratch dir %q", got, want)
	}
}

// TestDriverReportPath_TimestampAheadOfRandomComponent asserts the timestamp appears ahead of the
// random component in the filename.
func TestDriverReportPath_TimestampAheadOfRandomComponent(t *testing.T) {
	l := &lyxcwd.Location{}
	now := func() time.Time { return time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC) }
	stubRand := func() string { return "cafe" }

	got := driverReportPath(l, "self", now, stubRand)
	timestamp := now().Format(driverReportTimestampLayout)

	timestampIdx := strings.Index(got, timestamp)
	randIdx := strings.Index(got, "cafe")
	if timestampIdx < 0 || randIdx < 0 {
		t.Fatalf("driverReportPath() = %q; want it to contain both the timestamp %q and the random suffix %q", got, timestamp, "cafe")
	}
	if timestampIdx >= randIdx {
		t.Errorf("driverReportPath() = %q; want the timestamp %q ahead of the random component %q", got, timestamp, "cafe")
	}
}

// TestDriverReportPath_FixedStubRandMakesPathDeterministic asserts a fixed stub random source makes
// the whole path deterministic.
func TestDriverReportPath_FixedStubRandMakesPathDeterministic(t *testing.T) {
	l := &lyxcwd.Location{}
	now := func() time.Time { return time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC) }
	stubRand := func() string { return "dead" }

	got1 := driverReportPath(l, "self", now, stubRand)
	got2 := driverReportPath(l, "self", now, stubRand)
	if got1 != got2 {
		t.Errorf("driverReportPath() = %q, then %q; want identical paths from a fixed clock and a fixed rand seam", got1, got2)
	}
}

// TestNewDriverReportRand_ProducesFourHexChars sanity-checks the production random source's shape.
func TestNewDriverReportRand_ProducesFourHexChars(t *testing.T) {
	got := newDriverReportRand()()
	if len(got) != 4 {
		t.Errorf("newDriverReportRand()() = %q; want 4 characters", got)
	}
	for _, r := range got {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Errorf("newDriverReportRand()() = %q; want lowercase hex only", got)
			break
		}
	}
}

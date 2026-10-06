package loomcli

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestDriverReportPath asserts the composed report path under a fixed clock and rand seam.
//
//testtiming:keep pins the report path composed under the drive-reports directory with a timestamp ahead of the random part, differing across two calls under a frozen clock and deterministic under a fixed rand; the covering integration test only runs the composer
func TestDriverReportPath(t *testing.T) {
	t.Parallel()
	l := &lyxcwd.Location{}
	now := func() time.Time { return time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC) }

	// Two calls under a frozen clock must produce two different paths.
	// A test that instead advances the clock between calls would prove only that the timestamp varies,
	// the half that already worked, and would pass against a composer with no random component at all.
	// This is the only case that can fail against a composer missing the random half,
	// and the collision it guards fires precisely under the automated relaunch.
	t.Run("frozen clock, two calls produce different paths", func(t *testing.T) {
		t.Parallel()
		seq := []string{"aaaa", "bbbb"}
		i := 0
		stubRand := func() string {
			v := seq[i]
			i++
			return v
		}

		got1 := driverReportPath(l, "self", now, stubRand)
		got2 := driverReportPath(l, "self", now, stubRand)

		if got1 == got2 {
			t.Fatalf("driverReportPath() under a frozen clock produced the same path twice: %q -- a same-second relaunch would collide against Spec.validate's pre-existing-file refusal", got1)
		}
	})

	t.Run("lands directly under the run's drive-reports directory", func(t *testing.T) {
		t.Parallel()
		got := driverReportPath(l, "self", now, func() string { return "cafe" })
		want := shedrun.DriveReportsDir(l, "self")
		if parent := filepath.Dir(got); parent != want {
			t.Errorf("driverReportPath() = %q; want its parent to be the drive-reports dir %q, got %q", got, want, parent)
		}
	})

	t.Run("timestamp is ahead of the random component", func(t *testing.T) {
		t.Parallel()
		got := driverReportPath(l, "self", now, func() string { return "cafe" })
		timestamp := now().Format(driverReportTimestampLayout)

		timestampIdx := strings.Index(got, timestamp)
		randIdx := strings.Index(got, "cafe")
		if timestampIdx < 0 || randIdx < 0 {
			t.Fatalf("driverReportPath() = %q; want it to contain both the timestamp %q and the random suffix %q", got, timestamp, "cafe")
		}
		if timestampIdx >= randIdx {
			t.Errorf("driverReportPath() = %q; want the timestamp %q ahead of the random component %q", got, timestamp, "cafe")
		}
	})

	t.Run("a fixed stub rand makes the whole path deterministic", func(t *testing.T) {
		t.Parallel()
		stubRand := func() string { return "dead" }

		got1 := driverReportPath(l, "self", now, stubRand)
		got2 := driverReportPath(l, "self", now, stubRand)
		if got1 != got2 {
			t.Errorf("driverReportPath() = %q, then %q; want identical paths from a fixed clock and a fixed rand seam", got1, got2)
		}
	})
}

// TestNewDriverReportRand_ProducesFourHexChars sanity-checks the production random source's shape.
//
//testtiming:keep pins the production random source returning four lowercase hex characters; its covering test passes a stub source instead
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

// main_test.go checks tokencount's flag set: the retired history flag is refused.

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRefusesTheRetiredHistoryFlag(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := run([]string{"-history", "somewhere", "-calibrate", "fit", "slug"}, &out)
	if err == nil || !strings.Contains(err.Error(), "flag provided but not defined: -history") {
		t.Errorf("run error = %v; want the undefined-flag error naming -history", err)
	}
	if out.Len() != 0 {
		t.Errorf("run wrote %q; want no report", out.String())
	}
}

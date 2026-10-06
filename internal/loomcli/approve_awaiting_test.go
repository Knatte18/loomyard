package loomcli

import (
	"strings"
	"testing"
)

//testtiming:keep pins the approve command's Long help saying "awaiting or blocked at PR-Gate"; its covering tests build the command without asserting its help text
func TestApproveCmd_HelpNamesAwaitingOrBlocked(t *testing.T) {
	cmd := (&loomCLI{}).approveCmd()
	if !strings.Contains(cmd.Long, "awaiting or blocked at PR-Gate") {
		t.Errorf("Long %q does not say \"awaiting or blocked at PR-Gate\"", cmd.Long)
	}
}

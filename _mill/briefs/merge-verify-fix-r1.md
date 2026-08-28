# Verify-Fix Brief

The verify command `go test ./internal/shell/... ./internal/reedengine/...` failed after a merge.
Your job is to diagnose the failures and fix the code so the verify command passes.

## Verify Output

```
ok  	github.com/Knatte18/loomyard/internal/shell	(cached)
time=2026-08-28T13:58:35.257+02:00 level=WARN msg="reed: failed to query live window size, falling back to configured box" trace=bb02680a3d17e3fe socket=lyx-001-235456e5 session=worktree err=boom
time=2026-08-28T13:58:35.257+02:00 level=WARN msg="reed: malformed live window size answer, falling back to configured box" trace=bb02680a3d17e3fe socket=lyx-001-9121be7e session=worktree answer=""
time=2026-08-28T13:58:35.257+02:00 level=WARN msg="reed: malformed live window size answer, falling back to configured box" trace=bb02680a3d17e3fe socket=lyx-001-0c190118 session=worktree answer=""
time=2026-08-28T13:58:35.257+02:00 level=WARN msg="reed: malformed live window size answer, falling back to configured box" trace=bb02680a3d17e3fe socket=lyx-001-8928b830 session=worktree answer=""
time=2026-08-28T13:58:35.258+02:00 level=WARN msg="reed: failed to install resize-pane hook" trace=bb02680a3d17e3fe socket=lyx-001-8928b830 session=worktree err=boom
time=2026-08-28T13:58:35.259+02:00 level=WARN msg="reed: attach chain suppressed, attaching without a chained layout" trace=bb02680a3d17e3fe socket=lyx-001-0ef562a2 session=worktree err="attach chain suppressed"
time=2026-08-28T13:58:35.260+02:00 level=WARN msg="reed: attach chain suppressed, attaching without a chained layout" trace=bb02680a3d17e3fe socket=lyx-001-c5b7d1f6 session=worktree err="attach chain suppressed"
time=2026-08-28T13:58:35.260+02:00 level=WARN msg="reed: attach chain suppressed, attaching without a chained layout" trace=bb02680a3d17e3fe socket=lyx-001-16f387a8 session=worktree err="attach chain suppressed"
time=2026-08-28T13:58:35.260+02:00 level=WARN msg="reed: failed to read back window-size option" trace=bb02680a3d17e3fe socket=lyx-001-a7dab54a session=worktree err=boom
time=2026-08-28T13:58:35.260+02:00 level=WARN msg="reed: attach chain suppressed, attaching without a chained layout" trace=bb02680a3d17e3fe socket=lyx-001-a7dab54a session=worktree err="attach chain suppressed"
time=2026-08-28T13:58:35.260+02:00 level=WARN msg="reed: attach chain suppressed, attaching without a chained layout" trace=bb02680a3d17e3fe socket=lyx-001-6cd283b7 session=worktree err="attach chain suppressed"
time=2026-08-28T13:58:35.260+02:00 level=WARN msg="reed: failed to read back status option" trace=bb02680a3d17e3fe socket=lyx-001-ec8875d2 session=worktree err=boom
time=2026-08-28T13:58:35.260+02:00 level=WARN msg="reed: attach chain suppressed, attaching without a chained layout" trace=bb02680a3d17e3fe socket=lyx-001-ec8875d2 session=worktree err="attach chain suppressed"
time=2026-08-28T13:58:35.260+02:00 level=WARN msg="reed: no client terminal size available, attaching without a chained layout" trace=bb02680a3d17e3fe socket=lyx-001-bcc51dc8 session=worktree cols=0 rows=24
time=2026-08-28T13:58:35.261+02:00 level=WARN msg="reed: no client terminal size available, attaching without a chained layout" trace=bb02680a3d17e3fe socket=lyx-001-845989dc session=worktree cols=-1 rows=24
time=2026-08-28T13:58:35.261+02:00 level=WARN msg="reed: no client terminal size available, attaching without a chained layout" trace=bb02680a3d17e3fe socket=lyx-001-f7ffa767 session=worktree cols=80 rows=0
time=2026-08-28T13:58:35.261+02:00 level=WARN msg="reed: no client terminal size available, attaching without a chained layout" trace=bb02680a3d17e3fe socket=lyx-001-749258ab session=worktree cols=80 rows=-1
time=2026-08-28T13:58:35.261+02:00 level=WARN msg="reed: attach pre-flight failed, attaching without a chained layout" trace=bb02680a3d17e3fe socket=lyx-001-d7918d21 session=worktree err="check session: boom"
time=2026-08-28T13:58:35.261+02:00 level=WARN msg="reed: attach chain suppressed, attaching without a chained layout" trace=bb02680a3d17e3fe socket=lyx-001-3e1d6bc9 session=worktree err="attach chain suppressed"
time=2026-08-28T13:58:35.261+02:00 level=WARN msg="reed: attach chain suppressed, attaching without a chained layout" trace=bb02680a3d17e3fe socket=lyx-001-f25f125e session=worktree err="attach chain suppressed"
time=2026-08-28T13:58:35.261+02:00 level=WARN msg="reed: attach pre-flight failed, attaching without a chained layout" trace=bb02680a3d17e3fe socket=lyx-001-4dcf8861 session=worktree err=boom
time=2026-08-28T13:58:35.261+02:00 level=WARN msg="reed: attach pre-flight failed, attaching without a chained layout" trace=bb02680a3d17e3fe socket=lyx-001-5cd24d63 session=worktree err="render: strand a uses deferred anchor \"own-window\""
--- FAIL: TestAttachArgv_InstallsResizePinsAfterStateAndPanesRead (0.00s)
    attach_test.go:455: sequence = [has-session set-option:status set-option:window-size set-hook display-message:window-size display-message:status list-panes set-hook], want the first set-hook call (index 3) after list-panes (index 6)
    attach_test.go:461: first set-hook argv = [set-hook -t =worktree: window-resized run-shell -b ": > '/tmp/TestAttachArgv_InstallsResizePinsAfterStateAndPanesRead1859557878/001/worktree/anchor/.lyx/reed-resize.signal'"], want the -u clear
time=2026-08-28T13:58:35.262+02:00 level=WARN msg="reed: no client terminal size available, attaching without a chained layout" trace=bb02680a3d17e3fe socket=lyx-001-0c8f3539 session=worktree cols=0 rows=24
time=2026-08-28T13:58:35.262+02:00 level=WARN msg="reed: attach pre-flight failed, attaching without a chained layout" trace=bb02680a3d17e3fe socket=lyx-001-e440f859 session=worktree err="check session: boom"
time=2026-08-28T13:58:35.263+02:00 level=WARN msg="reed: attach chain suppressed, attaching without a chained layout" trace=bb02680a3d17e3fe socket=lyx-001-5f038fee session=worktree err="attach chain suppressed"
time=2026-08-28T13:58:35.263+02:00 level=WARN msg="reed: attach chain suppressed, attaching without a chained layout" trace=bb02680a3d17e3fe socket=lyx-001-ff600b4f session=worktree err="attach chain suppressed"
time=2026-08-28T13:58:35.263+02:00 level=WARN msg="reed: attach pre-flight failed, attaching without a chained layout" trace=bb02680a3d17e3fe socket=lyx-001-57a6239b session=worktree err="render: strand a uses deferred anchor \"own-window\""
--- FAIL: TestAttachArgv_DegradedPathsInstallNoResizePinHook (0.00s)
    --- FAIL: TestAttachArgv_DegradedPathsInstallNoResizePinHook/FewerThanTwoLivePanes (0.00s)
        attach_test.go:491: set-hook calls = [[set-hook -t =worktree: window-resized run-shell -b ": > '/tmp/TestAttachArgv_DegradedPathsInstallNoResizePinHookFewerThanTwoL3801956016/001/worktree/anchor/.lyx/reed-resize.signal'"]], want none
    --- FAIL: TestAttachArgv_DegradedPathsInstallNoResizePinHook/NoStrandOwnsAPresentPane (0.00s)
        attach_test.go:498: set-hook calls = [[set-hook -t =worktree: window-resized run-shell -b ": > '/tmp/TestAttachArgv_DegradedPathsInstallNoResizePinHookNoStrandOwnsA3034681680/001/worktree/anchor/.lyx/reed-resize.signal'"]], want none
    --- FAIL: TestAttachArgv_DegradedPathsInstallNoResizePinHook/PlanError_DeferredAnchorRejected (0.00s)
        attach_test.go:506: set-hook calls = [[set-hook -t =worktree: window-resized run-shell -b ": > '/tmp/TestAttachArgv_DegradedPathsInstallNoResizePinHookPlanError_Def3342974655/001/worktree/anchor/.lyx/reed-resize.signal'"]], want none
time=2026-08-28T13:58:35.263+02:00 level=WARN msg="reed: failed to install window-resized hook" trace=bb02680a3d17e3fe socket=lyx-001-8a66639d session=worktree err=boom
time=2026-08-28T13:58:35.263+02:00 level=WARN msg="reed: failed to install resize-pane hook" trace=bb02680a3d17e3fe socket=lyx-001-8a66639d session=worktree err=boom
time=2026-08-28T13:58:35.263+02:00 level=WARN msg="reed: persisted pane bindings were minted against a different tmux session incarnation, clearing them" trace=bb02680a3d17e3fe socket=lyx-001-686b6240 session=worktree recordedSession=worktree recordedTmuxSession=$0 recordedServerPID=100 liveTmuxSession=$4 liveServerPID=900
time=2026-08-28T13:58:35.263+02:00 level=WARN msg="reed: persisted pane bindings were minted against a different tmux session incarnation, clearing them" trace=bb02680a3d17e3fe socket=lyx-001-1bd0cf42 session=worktree recordedSession=svc-orig recordedTmuxSession=$0 recordedServerPID=100 liveTmuxSession=$4 liveServerPID=900
time=2026-08-28T13:58:35.264+02:00 level=WARN msg="reed: persisted pane bindings were minted against a different tmux session incarnation, clearing them" trace=bb02680a3d17e3fe socket=lyx-001-f20dcd31 session=worktree recordedSession=svc-orig recordedTmuxSession=$0 recordedServerPID=100 liveTmuxSession=$4 liveServerPID=900
time=2026-08-28T13:58:35.264+02:00 level=WARN msg="reed: could not read the live pane generation, leaving persisted pane bindings as they are" trace=bb02680a3d17e3fe socket=lyx-001-0b48a122 session=worktree err="tmux answered \"|4321|\" for session \"worktree\"; one of the 3 fields is empty"
time=2026-08-28T13:58:35.265+02:00 level=WARN msg="reed: failed to split header pane, retrying behind an even-vertical re-tile" trace=bb02680a3d17e3fe socket=lyx-001-47c21f6a session=worktree err="split-window created no new pane (got \"%0\"; target %0 likely too small to split)"
time=2026-08-28T13:58:35.265+02:00 level=WARN msg="reed: header split still had no room after the even-vertical re-tile" trace=bb02680a3d17e3fe socket=lyx-001-47c21f6a session=worktree err="split-window created no new pane (got \"%0\"; target %0 likely too small to split)"
time=2026-08-28T13:58:35.265+02:00 level=WARN msg="reed: failed to split header pane, retrying behind an even-vertical re-tile" trace=bb02680a3d17e3fe socket=lyx-001-e5b33343 session=worktree err="exit status 1: no space for new pane"
time=2026-08-28T13:58:35.266+02:00 level=WARN msg="reed: failed to split header pane, retrying behind an even-vertical re-tile" trace=bb02680a3d17e3fe socket=lyx-001-04f0624a session=worktree err="exit status 1: no space for new pane"
time=2026-08-28T13:58:35.420+02:00 level=WARN msg="reed: failed to query live window size, falling back to configured box" trace=bb02680a3d17e3fe socket=lyx-001-360e6d30 session=worktree err=boom
time=2026-08-28T13:58:35.420+02:00 level=WARN msg="reed: failed to query live window size, falling back to configured box" trace=bb02680a3d17e3fe socket=lyx-001-8f1cd076 session=worktree err=boom
time=2026-08-28T13:58:35.423+02:00 level=WARN msg="reed: could not read the live pane generation, leaving persisted pane bindings as they are" trace=bb02680a3d17e3fe socket=lyx-001-ab654d6e session=worktree err="read pane generation for session \"worktree\": fork/exec /tmp/TestLoadOrInitStateLocked_AbsentFileInitializesFromEngineIdentit2932476135/001/does-not-exist-tmux.exe: no such file or directory"
time=2026-08-28T13:58:35.423+02:00 level=WARN msg="reed: could not read the live pane generation, leaving persisted pane bindings as they are" trace=bb02680a3d17e3fe socket=lyx-001-f7a90052 session=worktree err="read pane generation for session \"worktree\": fork/exec /tmp/TestLoadOrInitStateLocked_ExistingFileLoadsStrandsAndRestampsIde3424442772/001/does-not-exist-tmux.exe: no such file or directory"
time=2026-08-28T13:58:35.424+02:00 level=WARN msg="reed: cleared strand pane bindings that named a pane another owner already claims" trace=bb02680a3d17e3fe socket=lyx-001-7bf52089 session=worktree strands=[first]
time=2026-08-28T13:58:35.424+02:00 level=WARN msg="reed: cleared strand pane bindings that named a pane another owner already claims" trace=bb02680a3d17e3fe socket=lyx-001-81291148 session=worktree strands=[second]
time=2026-08-28T13:58:35.660+02:00 level=WARN msg="reed: invalid watchdog value, treating watchdog as off" trace=bb02680a3d17e3fe socket=lyx-001-c8b1b900 session=worktree value=garbage err="invalid watchdog value \"garbage\": want \"on\" or \"off\""
time=2026-08-28T13:58:36.178+02:00 level=WARN msg="reed: resize re-apply failed" trace=bb02680a3d17e3fe socket=lyx-001-8133c3ef session=worktree err="select-layout: select-layout boom"
time=2026-08-28T13:58:36.183+02:00 level=WARN msg="reed: resize re-apply failed" trace=bb02680a3d17e3fe socket=lyx-001-8133c3ef session=worktree err="select-layout: select-layout boom"
time=2026-08-28T13:58:36.188+02:00 level=WARN msg="reed: resize re-apply failed" trace=bb02680a3d17e3fe socket=lyx-001-8133c3ef session=worktree err="select-layout: select-layout boom"
time=2026-08-28T13:58:36.188+02:00 level=WARN msg="reed: abandoning this resize event after max attempts, watcher remains running and responsive to the next signal" trace=bb02680a3d17e3fe socket=lyx-001-8133c3ef session=worktree attempts=3
time=2026-08-28T13:58:36.270+02:00 level=WARN msg="reed: malformed live window size answer, falling back to configured box" trace=bb02680a3d17e3fe socket=lyx-001-217eab12 session=worktree answer="abc def"
time=2026-08-28T13:58:36.270+02:00 level=WARN msg="reed: malformed live window size answer, falling back to configured box" trace=bb02680a3d17e3fe socket=lyx-001-e0116b21 session=worktree answer=""
time=2026-08-28T13:58:36.270+02:00 level=WARN msg="reed: malformed live window size answer, falling back to configured box" trace=bb02680a3d17e3fe socket=lyx-001-f1f46414 session=worktree answer="220 0"
time=2026-08-28T13:58:36.270+02:00 level=WARN msg="reed: failed to query live window size, falling back to configured box" trace=bb02680a3d17e3fe socket=lyx-001-3e255555 session=worktree err=boom
time=2026-08-28T13:58:36.271+02:00 level=WARN msg="reed: failed to read back status option" trace=bb02680a3d17e3fe socket=lyx-001-3b42e685 session=worktree err=boom
time=2026-08-28T13:58:36.271+02:00 level=WARN msg="reed: failed to read back window-size option" trace=bb02680a3d17e3fe socket=lyx-001-59b4160b session=worktree err=boom
time=2026-08-28T13:58:36.271+02:00 level=WARN msg="reed: failed to pin status off" trace=bb02680a3d17e3fe socket=lyx-001-dc521f0e session=worktree option=status err=boom
time=2026-08-28T13:58:36.271+02:00 level=WARN msg="reed: invalid watchdog value, treating the watchdog as off" trace=bb02680a3d17e3fe socket=lyx-001-48800905 session=worktree watchdog=bogus err="invalid watchdog value \"bogus\": want \"on\" or \"off\""
time=2026-08-28T13:58:36.271+02:00 level=WARN msg="reed: failed to install window-resized hook" trace=bb02680a3d17e3fe socket=lyx-001-41639fc0 session=worktree err=boom
FAIL
FAIL	github.com/Knatte18/loomyard/internal/reedengine	1.022s
ok  	github.com/Knatte18/loomyard/internal/reedengine/render	(cached)
FAIL
```

## Merge Diff

```diff
diff --git a/CONSTRAINTS.md b/CONSTRAINTS.md
index ac19ba413..89bcdefd0 100644
--- a/CONSTRAINTS.md
+++ b/CONSTRAINTS.md
@@ -101,6 +101,8 @@ Every producer prompt is read at call time from a told, absolute stencils direct
 - `//go:embed` in `contracts/stencils` is seed defaults only.
 - `internal/stencilstore` is sole owner of seeding/hashing/reading/validation. A hash-mismatched file is never overwritten.
 - Seed/refresh runs once per process pre-run, never lazily inside `Read`.
+  A command that reads no stencils may decline the pass entirely by carrying the skip annotation;
+  declining is all-or-nothing per command and never defers seeding to a later or lazier point.
 
 ## CLI / Cobra Invariant
 
diff --git a/cmd/lyx/main.go b/cmd/lyx/main.go
index c8629a450..91762ff13 100644
--- a/cmd/lyx/main.go
+++ b/cmd/lyx/main.go
@@ -83,7 +83,7 @@ Available modules: board, config, ide, reed, fabric, selfreport, shuttle, burler
 				logger.Arm()
 			}
 			// Seeding must never block a command from running, regardless of its outcome.
-			seedStencils(cmd.Context())
+			seedStencils(cmd)
 			return nil
 		},
 	}
diff --git a/cmd/lyx/stencilseed.go b/cmd/lyx/stencilseed.go
index 86359ae8b..b3bd74a90 100644
--- a/cmd/lyx/stencilseed.go
+++ b/cmd/lyx/stencilseed.go
@@ -15,8 +15,11 @@ import (
 	"path/filepath"
 	"testing"
 
+	"github.com/spf13/cobra"
+
 	"github.com/Knatte18/loomyard/contracts/stencils"
 	"github.com/Knatte18/loomyard/internal/buildinfo"
+	"github.com/Knatte18/loomyard/internal/clihelp"
 	"github.com/Knatte18/loomyard/internal/fabricengine"
 	"github.com/Knatte18/loomyard/internal/logger"
 	"github.com/Knatte18/loomyard/internal/lyxcwd"
@@ -26,7 +29,7 @@ import (
 
 // seedStencils is the thin pre-run wrapper newRoot's PersistentPreRunE calls: it resolves this
 // process' seed target via stencilSeedTarget and delegates to seedStencilsAt when one is found.
-func seedStencils(ctx context.Context) {
+func seedStencils(cmd *cobra.Command) {
 	// Return immediately under go test, before resolving anything: lyxcwd.Resolve spawns `git
 	// rev-parse --show-toplevel`, and cobra runs this root PersistentPreRunE for every Runnable
 	// command -- every parent group included, since each carries RunE: clihelp.GroupRunE. Without
@@ -37,13 +40,34 @@ func seedStencils(ctx context.Context) {
 		return
 	}
 
-	hub, worktree, ok := stencilSeedTarget(ctx)
+	// A command that carries the skip annotation reads no stencils, so the pass is pure waste for
+	// it -- and skipping also keeps a long-lived pane process (e.g. reed header's keepalive) from
+	// ever reaching fabricengine.CommitSeededStencils and performing a git commit in the hub. This
+	// early return sits ahead of stencilSeedTarget so an opted-out command resolves no geometry and
+	// spawns no `git rev-parse`.
+	if skipStencilSeed(cmd) {
+		return
+	}
+
+	hub, worktree, ok := stencilSeedTarget(cmd.Context())
 	if !ok {
 		return
 	}
 	seedStencilsAt(hub, worktree)
 }
 
+// skipStencilSeed reports whether cmd carries clihelp.SkipStencilSeedAnnotation set to
+// clihelp.AnnotationEnabled, and is therefore declining the root pre-run's stencil-seed pass.
+// It is extracted as its own directly-assertable function rather than inlined into seedStencils, for
+// the reason stencilSeedTarget's own comment already records: seedStencils returns immediately under
+// testing.Testing(), so a test can never observe the gate through it.
+func skipStencilSeed(cmd *cobra.Command) bool {
+	if cmd == nil {
+		return false
+	}
+	return cmd.Annotations[clihelp.SkipStencilSeedAnnotation] == clihelp.AnnotationEnabled
+}
+
 // stencilSeedTarget decides whether this process should seed stencils and, when it should, against
 // which hub and worktree.
 // It is a separate value-returning function -- rather than being inlined into seedStencils -- because
diff --git a/cmd/lyx/stencilseedgate_test.go b/cmd/lyx/stencilseedgate_test.go
new file mode 100644
index 000000000..0b3842f17
--- /dev/null
+++ b/cmd/lyx/stencilseedgate_test.go
@@ -0,0 +1,87 @@
+// stencilseedgate_test.go pins two things about the stencil-seed skip gate this batch adds:
+// skipStencilSeed's predicate against synthetic *cobra.Command values, and that "lyx reed header"
+// actually carries the annotation the predicate reads. It deliberately does NOT pin any ordering
+// between skipStencilSeed and stencilSeedTarget, and does NOT assert "no `git rev-parse` was
+// spawned": seedStencils returns under testing.Testing() before either step runs, so an in-process
+// test can never observe their relative order, and "no git rev-parse was spawned" is the unfalsifiable
+// shape the discussion driving this task rejected. Building the command tree here only constructs
+// cobra values and runs no hook, so nothing spawns a process and the Test Tier Purity Invariant holds.
+
+package main
+
+import (
+	"testing"
+
+	"github.com/spf13/cobra"
+
+	"github.com/Knatte18/loomyard/internal/clihelp"
+	"github.com/Knatte18/loomyard/internal/reedcli"
+)
+
+// TestSkipStencilSeed_HonoursTheAnnotation drives skipStencilSeed directly against synthetic
+// *cobra.Command values.
+func TestSkipStencilSeed_HonoursTheAnnotation(t *testing.T) {
+	tests := []struct {
+		name string
+		cmd  *cobra.Command
+		want bool
+	}{
+		{
+			name: "carries the enabled annotation",
+			cmd: &cobra.Command{
+				Annotations: map[string]string{clihelp.SkipStencilSeedAnnotation: clihelp.AnnotationEnabled},
+			},
+			want: true,
+		},
+		{
+			name: "no annotations map at all",
+			cmd:  &cobra.Command{},
+			want: false,
+		},
+		{
+			name: "an unrelated annotation key",
+			cmd: &cobra.Command{
+				Annotations: map[string]string{"some.other.key": clihelp.AnnotationEnabled},
+			},
+			want: false,
+		},
+		{
+			name: "the key present with value \"false\"",
+			cmd: &cobra.Command{
+				Annotations: map[string]string{clihelp.SkipStencilSeedAnnotation: "false"},
+			},
+			want: false,
+		},
+		{
+			name: "a nil command",
+			cmd:  nil,
+			want: false,
+		},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			if got := skipStencilSeed(tt.cmd); got != tt.want {
+				t.Errorf("skipStencilSeed(%+v) = %v; want %v", tt.cmd, got, tt.want)
+			}
+		})
+	}
+}
+
+// TestReedHeaderCarriesTheStencilSeedSkipAnnotation walks reedcli.Command()'s subcommands for
+// "header" and asserts it carries clihelp.SkipStencilSeedAnnotation set to clihelp.AnnotationEnabled.
+func TestReedHeaderCarriesTheStencilSeedSkipAnnotation(t *testing.T) {
+	var header *cobra.Command
+	for _, sub := range reedcli.Command().Commands() {
+		if sub.Name() == "header" {
+			header = sub
+			break
+		}
+	}
+	if header == nil {
+		t.Fatal("reedcli.Command() has no \"header\" subcommand")
+	}
+	if got := header.Annotations[clihelp.SkipStencilSeedAnnotation]; got != clihelp.AnnotationEnabled {
+		t.Errorf("reed header Annotations[%q] = %q; want %q -- the annotation was silently dropped, making the gate worthless",
+			clihelp.SkipStencilSeedAnnotation, got, clihelp.AnnotationEnabled)
+	}
+}
diff --git a/internal/clihelp/annotations.go b/internal/clihelp/annotations.go
new file mode 100644
index 000000000..79aea5244
--- /dev/null
+++ b/internal/clihelp/annotations.go
@@ -0,0 +1,18 @@
+// annotations.go declares the cobra command annotation keys internal/clihelp exposes for cmd/lyx's
+// root pre-run to consult. internal/clihelp is where this belongs: it already owns the CLI-wide seams
+// both cmd/lyx and every *cli package import, so this constant creates no new dependency edge in
+// either direction.
+
+package clihelp
+
+// SkipStencilSeedAnnotation is the cobra-annotation key a command carries to decline the root
+// pre-run's stencil-seed pass.
+// Declining is all-or-nothing per command: it is for a command that reads no stencils and is expected
+// to stay silent.
+// A value other than AnnotationEnabled never opts out -- so a "false" cannot silently read as an
+// opt-out.
+const SkipStencilSeedAnnotation = "lyx.skip-stencil-seed"
+
+// AnnotationEnabled is the one value that reads as "on" for any annotation key in this package,
+// SkipStencilSeedAnnotation included.
+const AnnotationEnabled = "true"
diff --git a/internal/clihelp/annotations_test.go b/internal/clihelp/annotations_test.go
new file mode 100644
index 000000000..1270dbe6e
--- /dev/null
+++ b/internal/clihelp/annotations_test.go
@@ -0,0 +1,21 @@
+// annotations_test.go asserts the exact literal values of this package's cobra-annotation constants,
+// so a rename cannot silently decouple a producer command (e.g. reed header) from the consumer gate
+// (cmd/lyx's skipStencilSeed).
+
+package clihelp
+
+import "testing"
+
+// TestSkipStencilSeedAnnotation_Literal pins SkipStencilSeedAnnotation's exact string value.
+func TestSkipStencilSeedAnnotation_Literal(t *testing.T) {
+	if got, want := SkipStencilSeedAnnotation, "lyx.skip-stencil-seed"; got != want {
+		t.Errorf("SkipStencilSeedAnnotation = %q; want %q", got, want)
+	}
+}
+
+// TestAnnotationEnabled_Literal pins AnnotationEnabled's exact string value.
+func TestAnnotationEnabled_Literal(t *testing.T) {
+	if got, want := AnnotationEnabled, "true"; got != want {
+		t.Errorf("AnnotationEnabled = %q; want %q", got, want)
+	}
+}
diff --git a/internal/reedcli/header.go b/internal/reedcli/header.go
index 6b652071c..7679b977c 100644
--- a/internal/reedcli/header.go
+++ b/internal/reedcli/header.go
@@ -2,7 +2,13 @@
 // tokenvocab-backed pipeline.
 // The default mode returns the rendered text through the normal JSON envelope;
 // --blocking prints the text then blocks forever, the one envelope-exempt tail this command has —
-// the header pane boots "lyx reed header --blocking" as its keepalive.
+// the header pane boots "lyx reed header --blocking" as its keepalive, running as the pane's own
+// command rather than being typed into a shell that could echo or leave other noise behind it.
+// Both modes carry clihelp.SkipStencilSeedAnnotation, declining cmd/lyx's root pre-run stencil-seed
+// pass: this is deliberate rather than a --blocking-only gate, because a cobra annotation is
+// per-command and neither mode reads a stencil. Declining is what keeps the keepalive's stderr — and
+// therefore the header pane's scrollback — free of stencilstore warnings, and the hub free of a
+// preview command's git commits.
 
 package reedcli
 
@@ -35,6 +41,23 @@ var headerWatch = func(ctx context.Context, eng *reedengine.Engine) error { retu
 // headerPark is the keepalive park the blocking tail ends on, unconditionally.
 var headerPark = blockForever
 
+// headerBlockingPayload returns the exact bytes the --blocking mode writes to the pane before it
+// blocks forever: an ED 2 + ED 3 + cursor-home escape sequence, followed by text with its trailing
+// carriage returns and newlines trimmed. It is split out as a pure helper, the same
+// composition-split-from-side-effecting-call-site shape internal/reedengine/headerpane.go uses,
+// so the byte sequence stays assertable without driving the --blocking path itself, which blocks
+// forever and never returns to a test.
+//
+// ED 2 (\x1b[2J) clears only the visible screen and does not touch the terminal's scrollback
+// buffer, which is precisely why shell/log noise written before this command runs could survive
+// where an operator eventually saw it. ED 3 (\x1b[3J) is a backstop: it clears the scrollback too,
+// guaranteeing the pane is clean at the moment the header renders regardless of what any future
+// code path, shell, or terminal wrote before it. It is not the pin for any individual source fix —
+// it is defence in depth that stays green even if one of those fixes regresses.
+func headerBlockingPayload(text string) string {
+	return "\x1b[2J\x1b[3J\x1b[H" + strings.TrimRight(text, "\r\n")
+}
+
 // headerCmd builds the `header` subcommand: calls c.eng.HeaderText() and either returns it via the JSON envelope or prints and blocks forever.
 func (c *reedCLI) headerCmd() *cobra.Command {
 	var blocking bool
@@ -47,10 +70,13 @@ func (c *reedCLI) headerCmd() *cobra.Command {
 Engine.ValidateHeader checks eagerly at boot.
 
 Default mode returns the rendered text through the normal JSON envelope —
-a plain, smoke-testable command. --blocking instead prints the rendered
-text to stdout and then blocks forever; this is the header pane's own
-keepalive tail and the one part of this command exempt from the JSON
-envelope (everything fallible still runs pre-flight, on the envelope).
+a plain, smoke-testable command. --blocking instead clears the pane's
+screen and scrollback, prints the rendered text to stdout, and then
+blocks forever; this is the header pane's own keepalive tail, run
+directly as the pane's command rather than typed into a shell that
+would survive it, and the one part of this command exempt from the
+JSON envelope (everything fallible still runs pre-flight, on the
+envelope).
 The blocking pane additionally runs reed's resize self-heal watch loop,
 which re-applies the planned layout after the terminal window is
 resized, and is turned off with "watchdog: off" in reed.yaml followed
@@ -65,6 +91,9 @@ next rebuilt (a server restart, a dead-header heal, or "lyx reed down" +
 Example:
   lyx reed header
   lyx reed header --blocking`,
+		Annotations: map[string]string{
+			clihelp.SkipStencilSeedAnnotation: clihelp.AnnotationEnabled,
+		},
 		RunE: func(cmd *cobra.Command, args []string) error {
 			if clihelp.ShouldAbort(cmd.Context()) {
 				return nil
@@ -79,7 +108,7 @@ Example:
 
 			if blocking {
 				// Display the rendered text once, then hold the pane open forever.
-				fmt.Fprint(out, "\x1b[2J\x1b[H"+strings.TrimRight(text, "\r\n"))
+				fmt.Fprint(out, headerBlockingPayload(text))
 
 				// The header pane's stdout/stderr is its visible screen: internal/logger's stderr half
 				// defaults to slog.LevelWarn, and the watch loop reaches already-shipped Warn call sites
diff --git a/internal/reedcli/header_test.go b/internal/reedcli/header_test.go
index 7da73da50..14529b772 100644
--- a/internal/reedcli/header_test.go
+++ b/internal/reedcli/header_test.go
@@ -1,6 +1,12 @@
 // header_test.go covers the `header` verb's pure command construction: Use, Short, and the
 // --blocking flag registration, plus the blocking tail's keepalive-survival contract, exercised
-// with headerWatch and headerPark both stubbed via card 19's package vars.
+// with headerWatch and headerPark both stubbed via package vars, and headerBlockingPayload's
+// clear-sequence bytes. It never runs RunE/PreRunE and never invokes the full --blocking path in
+// most tests, since that path blocks forever by design — the payload and contract are pinned
+// instead by driving the pure helpers directly and stubbing the blocking-tail components.
+// It also never drives the enveloped default through RunCLI: that reaches reed's PersistentPreRunE
+// and therefore lyxcwd.Resolve, which spawns "git rev-parse", banned in the untagged suite by the
+// Test Tier Purity Invariant.
 // The enveloped default's end-to-end PreRunE -> HeaderText round trip is covered by the reed smoke
 // suite (batch 4), not here.
 
@@ -170,3 +176,37 @@ type errHeaderWatchStub struct{}
 
 // Error implements the error interface with a fixed, test-only message.
 func (errHeaderWatchStub) Error() string { return "stub watch failure for test" }
+
+func TestHeaderBlockingPayload(t *testing.T) {
+	const clearSeq = "\x1b[2J\x1b[3J\x1b[H"
+
+	tests := []struct {
+		name string
+		text string
+		want string
+	}{
+		{
+			name: "TrailingCRLFTrimmed",
+			text: "hub: /some/path\r\n",
+			want: clearSeq + "hub: /some/path",
+		},
+		{
+			name: "NoTrailingNewlineUnchanged",
+			text: "hub: /some/path",
+			want: clearSeq + "hub: /some/path",
+		},
+		{
+			name: "InteriorNewlinesPreserved",
+			text: "line one\nline two\n\n",
+			want: clearSeq + "line one\nline two",
+		},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			got := headerBlockingPayload(tt.text)
+			if got != tt.want {
+				t.Errorf("headerBlockingPayload(%q) = %q; want %q", tt.text, got, tt.want)
+			}
+		})
+	}
+}
diff --git a/internal/reedcli/smoke_headerscrollback_test.go b/internal/reedcli/smoke_headerscrollback_test.go
new file mode 100644
index 000000000..f00837e97
--- /dev/null
+++ b/internal/reedcli/smoke_headerscrollback_test.go
@@ -0,0 +1,189 @@
+//go:build smoke
+
+// smoke_headerscrollback_test.go holds the two scrollback assertions this batch adds, both built on
+// capturePaneScrollback.
+// TestSmokeHeaderPayloadClearsPaneScrollback is the direct proof that the ED 3 backstop actually
+// clears a real multiplexer's scrollback — the one claim the composite test below can never show,
+// since it goes green either way once the source fixes land.
+// TestSmokeHeaderPaneScrollbackIsClean is the composite backstop B: it pins the end-to-end outcome
+// across boot, resume, and heal, and pins none of the individual source fixes — P1, P2, and P3 are
+// the pins for those, and they live in internal/reedengine/lifecycle_test.go,
+// internal/reedcli/smoke_headerseed_test.go, and internal/reedcli/header_test.go respectively.
+
+package reedcli
+
+import (
+	"bytes"
+	"fmt"
+	"os"
+	"os/exec"
+	"path/filepath"
+	"strings"
+	"testing"
+	"time"
+
+	"github.com/Knatte18/loomyard/contracts/stencils"
+	"github.com/Knatte18/loomyard/internal/fabricengine"
+	"github.com/Knatte18/loomyard/internal/hubforge"
+	"github.com/Knatte18/loomyard/internal/reedengine"
+	"github.com/Knatte18/loomyard/internal/stencilstore"
+)
+
+// TestSmokeHeaderPayloadClearsPaneScrollback proves ED 3 takes effect against a real multiplexer
+// rather than being a silent no-op: it fills a pane's scrollback with real junk lines, then emits
+// headerBlockingPayload's exact bytes into that same pane, and asserts the resulting scrollback
+// holds the header line and nothing else.
+// On a Windows/psmux host this claim is asserted, not verified — the discussion records both, but
+// this worktree is Linux and cannot execute a Windows run.
+func TestSmokeHeaderPayloadClearsPaneScrollback(t *testing.T) {
+	tmuxPath := tmuxBinaryPath(t)
+
+	tempDir := t.TempDir()
+	headerLine := "hub: " + tempDir
+
+	var payload strings.Builder
+	for i := 0; i < 50; i++ {
+		fmt.Fprintf(&payload, "smoke-junk-line-%03d\n", i)
+	}
+	payload.WriteString(headerBlockingPayload(headerLine))
+
+	payloadFile := filepath.Join(tempDir, "payload.txt")
+	if err := os.WriteFile(payloadFile, []byte(payload.String()), 0o644); err != nil {
+		t.Fatalf("write payload file %s: %v", payloadFile, err)
+	}
+
+	socket := fmt.Sprintf("lyx-headerscrollback-harness-%d", os.Getpid())
+	sessionCmd := fmt.Sprintf("cat %s; sleep 300", payloadFile)
+	if err := exec.Command(tmuxPath, "-L", socket, "new-session", "-d", "-s", "h",
+		"sh", "-c", sessionCmd).Run(); err != nil {
+		t.Fatalf("boot harness server: %v", err)
+	}
+	t.Cleanup(func() {
+		reapHarnessServer(t, tmuxPath, socket)
+	})
+
+	var capture string
+	deadline := time.Now().Add(20 * time.Second)
+	for {
+		capture = capturePaneScrollback(t, tmuxPath, socket, "h")
+		if strings.Contains(capture, headerLine) {
+			break
+		}
+		if time.Now().After(deadline) {
+			t.Fatalf("pane never showed %q within 20s; last scrollback:\n%s", headerLine, capture)
+		}
+		time.Sleep(200 * time.Millisecond)
+	}
+
+	if !strings.Contains(capture, headerLine) {
+		t.Errorf("scrollback missing header line %q; full capture:\n%s", headerLine, capture)
+	}
+	for i := 0; i < 50; i++ {
+		junk := fmt.Sprintf("smoke-junk-line-%03d", i)
+		if strings.Contains(capture, junk) {
+			t.Errorf("scrollback still contains junk line %q that ED 3 should have cleared; full capture:\n%s", junk, capture)
+		}
+	}
+}
+
+// TestSmokeHeaderPaneScrollbackIsClean is the composite backstop B: it pins the end-to-end
+// outcome — the live header pane's scrollback holds the rendered header line and no other
+// non-empty line — across boot, resume, and heal.
+// It pins no individual source fix: ED 3 runs after everything else and would keep this test green
+// even if a source fix regressed, which is exactly why it is landed only alongside the direct proof
+// above and the three per-mechanism pins (P1, P2, P3) elsewhere in this package and reedengine.
+func TestSmokeHeaderPaneScrollbackIsClean(t *testing.T) {
+	tmuxPath := tmuxBinaryPath(t)
+	lyxExe := buildLyxBinaryWithLDFlags(t, "-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev")
+
+	h := hubforge.NewHub(t, ".")
+	deferHubRelease(t, h.PrimeWorktree())
+	t.Chdir(h.PrimeWorktree())
+	t.Cleanup(func() {
+		var buf bytes.Buffer
+		RunCLI(&buf, []string{"down"})
+	})
+
+	// Plant the same stale-but-untouched board stencil TestSmokeHeaderDeclinesStencilSeedPass
+	// plants, so the arrangement that made the noise non-deterministic in the field is forced
+	// rather than hoped for.
+	registry := stencils.Registry()
+	names := registry.Names()
+	if len(names) == 0 {
+		t.Fatalf("stencils.Registry().Names() returned no names")
+	}
+	name := names[0]
+	shipped, known := registry.Default(name)
+	if !known {
+		t.Fatalf("registry has no default for its own first name %q", name)
+	}
+	driftedBody := append(append([]byte{}, shipped...), []byte("\nsmoke-drift-line\n")...)
+	boardPath := stencilstore.Path(fabricengine.StencilsDir(h.Path), name)
+	if err := os.MkdirAll(filepath.Dir(boardPath), 0o755); err != nil {
+		t.Fatalf("create board stencil parent dir: %v", err)
+	}
+	stamped := stencilstore.ApplyStamp(driftedBody, stencilstore.BodyHash(driftedBody))
+	if err := os.WriteFile(boardPath, stamped, 0o644); err != nil {
+		t.Fatalf("write board stencil %s: %v", boardPath, err)
+	}
+
+	headerLine := "hub: " + h.Location.HubPath
+	assertScrollbackClean := func(when, socket, paneID string) {
+		t.Helper()
+		capture := capturePaneScrollback(t, tmuxPath, socket, paneID)
+		if !strings.Contains(capture, headerLine) {
+			t.Errorf("%s: header pane scrollback missing %q; full capture:\n%s", when, headerLine, capture)
+		}
+		for _, line := range strings.Split(capture, "\n") {
+			line = strings.TrimRight(line, "\r")
+			if strings.TrimSpace(line) == "" || strings.Contains(line, headerLine) {
+				continue
+			}
+			t.Errorf("%s: header pane scrollback carries an unexpected non-empty line %q; full capture:\n%s", when, line, capture)
+		}
+	}
+	pollHeaderLine := func(socket, paneID string) {
+		t.Helper()
+		pollPaneContains(t, tmuxPath, socket, paneID, headerLine, 20*time.Second)
+	}
+
+	// Boot.
+	upCmd := exec.Command(lyxExe, "reed", "up")
+	upCmd.Dir = h.PrimeWorktree()
+	if out, err := upCmd.CombinedOutput(); err != nil {
+		t.Fatalf("built-binary up: %v\n%s", err, out)
+	}
+	st, err := reedengine.LoadState(filepath.Join(h.PrimeWorktree(), ".lyx"))
+	if err != nil || st == nil || st.HeaderPaneID == "" {
+		t.Fatalf("LoadState after up = (%+v, %v), want a persisted HeaderPaneID", st, err)
+	}
+	socket, _ := socketAndSession(t)
+	pollHeaderLine(socket, st.HeaderPaneID)
+	assertScrollbackClean("after up", socket, st.HeaderPaneID)
+
+	// Resume: already-live header pane must be left untouched, and its scrollback must stay clean.
+	resumeCmd := exec.Command(lyxExe, "reed", "resume")
+	resumeCmd.Dir = h.PrimeWorktree()
+	if out, err := resumeCmd.CombinedOutput(); err != nil {
+		t.Fatalf("built-binary resume: %v\n%s", err, out)
+	}
+	assertScrollbackClean("after resume", socket, st.HeaderPaneID)
+
+	// Heal: kill the header pane directly through tmux, then re-run up, which is the retried-split
+	// heal path — the one most likely to regress, since it re-runs the same launch code from a
+	// different entry point.
+	if err := exec.Command(tmuxPath, "-L", socket, "kill-pane", "-t", st.HeaderPaneID).Run(); err != nil {
+		t.Fatalf("kill-pane %s: %v", st.HeaderPaneID, err)
+	}
+	healCmd := exec.Command(lyxExe, "reed", "up")
+	healCmd.Dir = h.PrimeWorktree()
+	if out, err := healCmd.CombinedOutput(); err != nil {
+		t.Fatalf("built-binary up (heal): %v\n%s", err, out)
+	}
+	healedSt, err := reedengine.LoadState(filepath.Join(h.PrimeWorktree(), ".lyx"))
+	if err != nil || healedSt == nil || healedSt.HeaderPaneID == "" {
+		t.Fatalf("LoadState after heal up = (%+v, %v), want a fresh persisted HeaderPaneID", healedSt, err)
+	}
+	pollHeaderLine(socket, healedSt.HeaderPaneID)
+	assertScrollbackClean("after heal", socket, healedSt.HeaderPaneID)
+}
diff --git a/internal/reedcli/smoke_headerseed_test.go b/internal/reedcli/smoke_headerseed_test.go
new file mode 100644
index 000000000..b849d4d30
--- /dev/null
+++ b/internal/reedcli/smoke_headerseed_test.go
@@ -0,0 +1,88 @@
+//go:build smoke
+
+// smoke_headerseed_test.go pins noise class 3's suppression directly: TestSmokeHeaderDeclinesStencilSeedPass
+// arranges both stencilstore Warn emitters cmd/lyx's root PersistentPreRunE can reach (the dev-refusal
+// warn and the port-back drift warn) and asserts that `lyx reed header`'s stderr is empty. No tmux,
+// pane, or escape sequence is anywhere in this picture: the assertion runs the built binary as a plain
+// subprocess and reads its own stderr stream directly, so this test is structurally incapable of being
+// masked by the `ED 3` scrollback backstop batch 3 adds — that backstop clears a pane's scrollback, and
+// there is no pane here for it to touch.
+
+package reedcli
+
+import (
+	"bytes"
+	"os"
+	"os/exec"
+	"path/filepath"
+	"testing"
+
+	"github.com/Knatte18/loomyard/contracts/stencils"
+	"github.com/Knatte18/loomyard/internal/fabricengine"
+	"github.com/Knatte18/loomyard/internal/hubforge"
+	"github.com/Knatte18/loomyard/internal/stencilstore"
+)
+
+// TestSmokeHeaderDeclinesStencilSeedPass is P2, the batch's regression pin: a dev-stamped real binary
+// runs `lyx reed header` against a hub carrying both a stale-but-untouched board stencil (the
+// dev-refusal warn's precondition) and a drifted contracts/stencils worktree copy (the port-back
+// drift warn's precondition), and stderr must come back empty.
+// Assert emptiness only -- never a line count and never a particular message: either emitter alone is
+// enough to make stderr non-empty pre-fix, and post-fix stderr is silent because the pass does not run
+// at all for an opted-out command.
+func TestSmokeHeaderDeclinesStencilSeedPass(t *testing.T) {
+	lyxExe := buildLyxBinaryWithLDFlags(t, "-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev")
+
+	h := hubforge.NewHub(t, ".")
+	deferHubRelease(t, h.PrimeWorktree())
+
+	registry := stencils.Registry()
+	names := registry.Names()
+	if len(names) == 0 {
+		t.Fatalf("stencils.Registry().Names() returned no names")
+	}
+	name := names[0]
+
+	// Arrange the dev-refusal warn: a stamp matching its own body plus a body differing from the
+	// shipped default is exactly StateUntouched with drift, which reconcileOne warns about and
+	// refuses to refresh under ModeDev.
+	shipped, known := registry.Default(name)
+	if !known {
+		t.Fatalf("registry has no default for its own first name %q", name)
+	}
+	driftedBody := append(append([]byte{}, shipped...), []byte("\nsmoke-drift-line\n")...)
+	boardPath := stencilstore.Path(fabricengine.StencilsDir(h.Path), name)
+	if err := os.MkdirAll(filepath.Dir(boardPath), 0o755); err != nil {
+		t.Fatalf("create board stencil parent dir: %v", err)
+	}
+	stamped := stencilstore.ApplyStamp(driftedBody, stencilstore.BodyHash(driftedBody))
+	if err := os.WriteFile(boardPath, stamped, 0o644); err != nil {
+		t.Fatalf("write board stencil %s: %v", boardPath, err)
+	}
+
+	// Arrange the port-back drift warn: a hubforge fixture worktree carries no contracts/ directory
+	// of its own, so seedStencilsAt sets sourceDir empty and warnPortBackDrift cannot fire without
+	// this step -- materialize contracts/stencils/<relPath> with a body differing from the board copy
+	// just planted.
+	sourcePath := filepath.Join(h.PrimeWorktree(), "contracts", "stencils", filepath.FromSlash(stencilstore.RelPath(name)))
+	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
+		t.Fatalf("create contracts/stencils parent dir: %v", err)
+	}
+	sourceBody := append(append([]byte{}, shipped...), []byte("\nsmoke-source-drift-line\n")...)
+	if err := os.WriteFile(sourcePath, sourceBody, 0o644); err != nil {
+		t.Fatalf("write contracts/stencils source %s: %v", sourcePath, err)
+	}
+
+	cmd := exec.Command(lyxExe, "reed", "header")
+	cmd.Dir = h.PrimeWorktree()
+	var stdout, stderr bytes.Buffer
+	cmd.Stdout = &stdout
+	cmd.Stderr = &stderr
+	if err := cmd.Run(); err != nil {
+		t.Fatalf("lyx reed header: %v; stdout: %s; stderr: %s", err, stdout.String(), stderr.String())
+	}
+
+	if stderr.Len() != 0 {
+		t.Errorf("lyx reed header stderr = %q; want empty -- the header must decline the root pre-run's stencil-seed pass entirely", stderr.String())
+	}
+}
diff --git a/internal/reedcli/smoke_test.go b/internal/reedcli/smoke_test.go
index 59624f7ea..a30cb6c6b 100644
--- a/internal/reedcli/smoke_test.go
+++ b/internal/reedcli/smoke_test.go
@@ -658,6 +658,20 @@ func capturePane(t *testing.T, tmuxPath, socket, target string) string {
 	return string(out)
 }
 
+// capturePaneScrollback returns the target pane's full scrollback (via -S -), not merely its
+// visible viewport.
+// This is deliberately a separate helper from capturePane rather than an edit to it: capturePane
+// passes no -S and captures the visible viewport only, which is what its existing callers assert
+// against, whereas the header-noise assertions need the full scrollback that -S - reaches.
+func capturePaneScrollback(t *testing.T, tmuxPath, socket, target string) string {
+	t.Helper()
+	out, err := exec.Command(tmuxPath, "-L", socket, "capture-pane", "-p", "-S", "-", "-t", target).Output()
+	if err != nil {
+		t.Fatalf("capture-pane -S - -t %s: %v", target, err)
+	}
+	return string(out)
+}
+
 // sendKeysLine types text literally into the target pane and submits it with Enter.
 func sendKeysLine(t *testing.T, tmuxPath, socket, target, text string) {
 	t.Helper()
@@ -690,13 +704,28 @@ var _, smokeTestFile, _, _ = runtime.Caller(0)
 
 // buildLyxBinary compiles cmd/lyx into a temp dir and returns its path.
 func buildLyxBinary(t *testing.T) string {
+	t.Helper()
+	return buildLyxBinaryWithLDFlags(t, "")
+}
+
+// buildLyxBinaryWithLDFlags compiles cmd/lyx into a temp dir with the given -ldflags value (omitted
+// from the build argv entirely when ldflags is empty) and returns its path.
+// The dev channel stamp -X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev is what makes
+// stencilstore.ModeFor(buildinfo.IsDev()) return ModeDev: buildinfo.Channel is "" for a plain `go
+// build`, so an unstamped binary is production mode and never emits the dev-refusal warn.
+func buildLyxBinaryWithLDFlags(t *testing.T, ldflags string) string {
 	t.Helper()
 	repoRoot, err := filepath.Abs(filepath.Join(filepath.Dir(smokeTestFile), "..", ".."))
 	if err != nil {
 		t.Fatalf("resolve repo root: %v", err)
 	}
 	lyxExe := filepath.Join(t.TempDir(), "lyx.exe")
-	cmd := exec.Command("go", "build", "-o", lyxExe, "./cmd/lyx")
+	args := []string{"build", "-o", lyxExe}
+	if ldflags != "" {
+		args = append(args, "-ldflags", ldflags)
+	}
+	args = append(args, "./cmd/lyx")
+	cmd := exec.Command("go", args...)
 	cmd.Dir = repoRoot
 	if out, err := cmd.CombinedOutput(); err != nil {
 		t.Fatalf("go build ./cmd/lyx: %v\n%s", err, out)
diff --git a/internal/reedengine/apply.go b/internal/reedengine/apply.go
index 50f9f6a8b..7d711beff 100644
--- a/internal/reedengine/apply.go
+++ b/internal/reedengine/apply.go
@@ -62,11 +62,46 @@ func paneIDsByTop(live []LivePane) []string {
 	return ids
 }
 
+// renderInputs is the single mapping from persisted state plus the live pane set down to the
+// arguments the render package takes: the strand table, the height-policy params (including the
+// header, blanked when its pane is no longer present), and the physical pane order. Both planLayout
+// and fixedHeightPins are built on toRenderInputs and never compute this mapping themselves, so the
+// two can never disagree about which header id — or which strand set — they are laying out.
+type renderInputs struct {
+	strands   []render.Strand
+	params    render.Params
+	paneOrder []string
+}
+
+// toRenderInputs performs the persisted-state-to-render mapping exactly once: it filters st.Strands
+// to the present pane set, blanks st.HeaderPaneID when the header pane is not present, assembles the
+// render.Params this engine's config implies, and orders live's pane ids top to bottom. It touches no
+// tmux and queries nothing of its own — box and live are told to it by the caller, matching
+// planLayout's own told-box contract.
+func (e *Engine) toRenderInputs(st *ReedState, live []LivePane) renderInputs {
+	presentIDs := liveIDSet(live)
+	strands := toRenderStrands(st.Strands, presentIDs)
+	headerPaneID := st.HeaderPaneID
+	if !presentIDs[headerPaneID] {
+		headerPaneID = ""
+	}
+	return renderInputs{
+		strands: strands,
+		params: render.Params{
+			CollapsedStripRows: e.cfg.CollapsedStripRows,
+			MinFullRows:        e.cfg.MinFullRows,
+			Header:             render.Header{PaneID: headerPaneID, HeightRows: e.cfg.Header.HeightRows},
+		},
+		paneOrder: paneIDsByTop(live),
+	}
+}
+
 // planLayout computes the tmux window_layout string and focus pane id for
 // st's current strand table against live, within box, without touching tmux
 // itself: box is always told to it by the caller, and it queries nothing of
-// its own. It injects the header pane if present and filters by live panes
-// only.
+// its own. The persisted-state-to-render mapping lives in toRenderInputs,
+// which fixedHeightPins below shares, so the layout and the pin path can
+// never be computed from a different header id than each other.
 //
 // The two callers pass two different box sources: applyLayoutLocked passes
 // e.liveBoxLocked()'s live tmux window query (falling back to the configured
@@ -74,17 +109,19 @@ func paneIDsByTop(live []LivePane) []string {
 // the attaching client's own told terminal size and never calls
 // liveBoxLocked — see the Shared Decision told-box-wins-live-query-is-the-fallback.
 func (e *Engine) planLayout(st *ReedState, live []LivePane, box render.Box) (layout, focus string, err error) {
-	presentIDs := liveIDSet(live)
-	strands := toRenderStrands(st.Strands, presentIDs)
-	headerPaneID := st.HeaderPaneID
-	if !presentIDs[headerPaneID] {
-		headerPaneID = ""
-	}
-	return render.Rules(strands, box, render.Params{
-		CollapsedStripRows: e.cfg.CollapsedStripRows,
-		MinFullRows:        e.cfg.MinFullRows,
-		Header:             render.Header{PaneID: headerPaneID, HeightRows: e.cfg.Header.HeightRows},
-	}, paneIDsByTop(live))
+	in := e.toRenderInputs(st, live)
+	return render.Rules(in.strands, box, in.params, in.paneOrder)
+}
+
+// fixedHeightPins reports the panes whose heights are absolute row budgets — the header band and
+// every collapsed strip — for st's current strand table against live, within box. It calls
+// toRenderInputs and queries nothing of its own: box is told to it by the caller exactly as
+// planLayout is, and it must always be called with the same st, live and box triple the layout for
+// that same call was planned from, so the pins it returns never disagree with what was actually laid
+// out.
+func (e *Engine) fixedHeightPins(st *ReedState, live []LivePane, box render.Box) []render.Pin {
+	in := e.toRenderInputs(st, live)
+	return render.FixedHeightPins(in.strands, box, in.params)
 }
 
 // anyPlacedStrand reports whether at least one strand would be placed by
@@ -195,6 +232,7 @@ func (e *Engine) applyLayoutLockedOpts(st *ReedState, live []LivePane, opts appl
 	if opts.SkipFocus {
 		return applyResult{Applied: true, Box: box, BoxIsLive: boxIsLive}, nil
 	}
+	e.installResizePinsLocked(e.fixedHeightPins(st, live, box))
 	if focus == "" {
 		return applyResult{Applied: true, Box: box, BoxIsLive: boxIsLive}, nil
 	}
diff --git a/internal/reedengine/apply_test.go b/internal/reedengine/apply_test.go
index ef61891c5..8c6cae92e 100644
--- a/internal/reedengine/apply_test.go
+++ b/internal/reedengine/apply_test.go
@@ -462,6 +462,187 @@ func TestAnyPlacedStrand(t *testing.T) {
 	}
 }
 
+// applyHookRecorder captures every call applyLayoutLocked issues through the execHook seam, in order,
+// so a test can discriminate on the recorded call sequence rather than on call count alone. setHookArgvs
+// holds the full argv of every set-hook call, in call order, alongside sequence's "set-hook" entries.
+type applyHookRecorder struct {
+	sequence     []string
+	setHookArgvs [][]string
+}
+
+// newApplyRecordingHook builds the execHook closure a test installs on e.tmux, recording every call
+// into rec and answering select-layout/select-pane/set-hook with success — the fixture apply_test.go
+// lacked before this card, built from scratch rather than extending an existing single-purpose
+// closure.
+func newApplyRecordingHook(rec *applyHookRecorder) func(capture bool, args ...string) (string, error) {
+	return func(capture bool, args ...string) (string, error) {
+		switch args[0] {
+		case "select-layout":
+			rec.sequence = append(rec.sequence, "select-layout")
+			return "", nil
+		case "select-pane":
+			rec.sequence = append(rec.sequence, "select-pane")
+			return "", nil
+		case "set-hook":
+			rec.sequence = append(rec.sequence, "set-hook")
+			rec.setHookArgvs = append(rec.setHookArgvs, append([]string{}, args...))
+			return "", nil
+		default:
+			return "", nil
+		}
+	}
+}
+
+// TestApplyLayoutLocked_InstallsResizePinsAfterSelectLayout pins the install statement's position: a
+// successful apply issues the set-hook clear and pin rebuild after select-layout and before
+// select-pane, discriminated on the recorded call sequence.
+func TestApplyLayoutLocked_InstallsResizePinsAfterSelectLayout(t *testing.T) {
+	e := newTestEngine(t)
+	e.cfg.Width, e.cfg.Height = 100, 21
+	e.cfg.CollapsedStripRows, e.cfg.MinFullRows = 2, 3
+	e.cfg.Header.HeightRows = 1
+
+	rec := &applyHookRecorder{}
+	e.tmux.execHook = newApplyRecordingHook(rec)
+
+	st := &ReedState{
+		HeaderPaneID: "%9",
+		Strands: []Strand{
+			{GUID: "root", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent, ShrinkWhenWaitingOnChild: true}},
+			{GUID: "child", Parent: "root", PaneID: "%2", Display: render.Display{Anchor: render.AnchorBelowParent, Focus: true}},
+		},
+	}
+	live := []LivePane{{ID: "%9", Top: 0}, {ID: "%1", Top: 2}, {ID: "%2", Top: 4}}
+
+	if err := e.applyLayoutLocked(st, live); err != nil {
+		t.Fatalf("applyLayoutLocked() unexpected error: %v", err)
+	}
+
+	wantMinLen := 3 // select-layout, at least the set-hook clear, select-pane
+	if len(rec.sequence) < wantMinLen {
+		t.Fatalf("sequence = %v, want at least %d entries", rec.sequence, wantMinLen)
+	}
+	if rec.sequence[0] != "select-layout" {
+		t.Fatalf("sequence[0] = %q, want select-layout", rec.sequence[0])
+	}
+	if rec.sequence[1] != "set-hook" {
+		t.Fatalf("sequence[1] = %q, want set-hook (the install statement right after select-layout)", rec.sequence[1])
+	}
+	if rec.sequence[len(rec.sequence)-1] != "select-pane" {
+		t.Fatalf("sequence tail = %q, want select-pane after every set-hook call", rec.sequence[len(rec.sequence)-1])
+	}
+	for _, step := range rec.sequence[1 : len(rec.sequence)-1] {
+		if step != "set-hook" {
+			t.Errorf("sequence = %v, want only set-hook calls between select-layout and select-pane", rec.sequence)
+		}
+	}
+
+	if len(rec.setHookArgvs) == 0 {
+		t.Fatal("no set-hook calls recorded, want at least the clear")
+	}
+	clear := rec.setHookArgvs[0]
+	if containsArg(clear, "-u") == false {
+		t.Errorf("first set-hook argv = %v, want the -u clear", clear)
+	}
+}
+
+// TestApplyLayoutLocked_ZeroPinsStillIssuesTheClear pins the-clear-is-unconditional-including-zero-pins:
+// an apply whose plan yields zero pins — a HeaderPaneID absent from the live set, no strip strand
+// present — still issues the clear and nothing after it.
+func TestApplyLayoutLocked_ZeroPinsStillIssuesTheClear(t *testing.T) {
+	e := newTestEngine(t)
+	e.cfg.Width, e.cfg.Height = 100, 21
+	e.cfg.CollapsedStripRows, e.cfg.MinFullRows = 2, 3
+	e.cfg.Header.HeightRows = 1
+
+	rec := &applyHookRecorder{}
+	e.tmux.execHook = newApplyRecordingHook(rec)
+
+	st := &ReedState{
+		HeaderPaneID: "%9", // absent from live below, so the mapping blanks it
+		Strands: []Strand{
+			{GUID: "root", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
+			{GUID: "child", Parent: "root", PaneID: "%2", Display: render.Display{Anchor: render.AnchorBelowParent}},
+		},
+	}
+	live := []LivePane{{ID: "%1", Top: 0}, {ID: "%2", Top: 11}}
+
+	if err := e.applyLayoutLocked(st, live); err != nil {
+		t.Fatalf("applyLayoutLocked() unexpected error: %v", err)
+	}
+
+	setHookCount := 0
+	for _, step := range rec.sequence {
+		if step == "set-hook" {
+			setHookCount++
+		}
+	}
+	if setHookCount != 1 {
+		t.Fatalf("recorded %d set-hook calls, want exactly 1 (the unconditional clear): %v", setHookCount, rec.setHookArgvs)
+	}
+	if !containsArg(rec.setHookArgvs[0], "-u") {
+		t.Errorf("sole set-hook argv = %v, want the -u clear", rec.setHookArgvs[0])
+	}
+}
+
+// TestApplyLayoutLocked_GuardSkipIssuesNoSetHookCall pins guard-skip-leaves-a-stale-array-deliberately:
+// neither of applyLayoutLocked's two guards reaches any set-hook call at all, INCLUDING no clear, so a
+// previously installed array survives a guard-skip.
+func TestApplyLayoutLocked_GuardSkipIssuesNoSetHookCall(t *testing.T) {
+	e := newTestEngine(t)
+
+	t.Run("FewerThanTwoLivePanes", func(t *testing.T) {
+		rec := &applyHookRecorder{}
+		e.tmux.execHook = newApplyRecordingHook(rec)
+		st := &ReedState{Strands: []Strand{{GUID: "only", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}}}}
+
+		if err := e.applyLayoutLocked(st, []LivePane{{ID: "%1"}}); err != nil {
+			t.Fatalf("applyLayoutLocked() unexpected error: %v", err)
+		}
+		if len(rec.sequence) != 0 {
+			t.Errorf("sequence = %v, want zero calls (including no clear)", rec.sequence)
+		}
+	})
+
+	t.Run("NoStrandOwnsAPresentPane", func(t *testing.T) {
+		rec := &applyHookRecorder{}
+		e.tmux.execHook = newApplyRecordingHook(rec)
+		st := &ReedState{}
+
+		if err := e.applyLayoutLocked(st, []LivePane{{ID: "%1"}, {ID: "%2"}}); err != nil {
+			t.Fatalf("applyLayoutLocked() unexpected error: %v", err)
+		}
+		if len(rec.sequence) != 0 {
+			t.Errorf("sequence = %v, want zero calls (including no clear)", rec.sequence)
+		}
+	})
+}
+
+// TestApplyLayoutLocked_SetHookErrorDoesNotFailApply pins hook-failure-is-non-fatal-everywhere: a
+// set-hook returning an error does not make applyLayoutLocked return an error.
+func TestApplyLayoutLocked_SetHookErrorDoesNotFailApply(t *testing.T) {
+	e := newTestEngine(t)
+	e.cfg.Width, e.cfg.Height = 100, 21
+	e.cfg.CollapsedStripRows, e.cfg.MinFullRows = 2, 3
+
+	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
+		if args[0] == "set-hook" {
+			return "", errors.New("boom")
+		}
+		return "", nil
+	}
+
+	st := &ReedState{Strands: []Strand{
+		{GUID: "a", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
+		{GUID: "b", PaneID: "%2", Display: render.Display{Anchor: render.AnchorBelowParent}},
+	}}
+	live := []LivePane{{ID: "%1"}, {ID: "%2"}}
+
+	if err := e.applyLayoutLocked(st, live); err != nil {
+		t.Fatalf("applyLayoutLocked() = %v, want nil even when set-hook fails", err)
+	}
+}
+
 func TestPaneIDsByTop_SortsByVerticalPosition(t *testing.T) {
 	live := []LivePane{
 		{ID: "%3", Top: 32},
diff --git a/internal/reedengine/attach.go b/internal/reedengine/attach.go
index 27f790486..440abe258 100644
--- a/internal/reedengine/attach.go
+++ b/internal/reedengine/attach.go
@@ -59,6 +59,13 @@ func chainedAttachArgv(socket, session, layout string) []string {
 // cols and rows are the attaching client's own terminal size, in columns and rows; a non-positive
 // value means no client size is known, and AttachArgv returns the bare argv immediately without
 // taking the lock.
+//
+// The pre-flight also refreshes the session's window-resized resize-pin hook, computed against the
+// same told box the chained layout is. This is what corrects a later client resize, and — on a
+// session whose earlier apply already installed the hook — a degraded bare attach too. A degrade
+// return installs nothing: the uncovered window is a session between "up" and its first placed
+// strand, which has nothing to pin anyway because a lone header pane takes render.Rules' sole-cell
+// branch.
 func (e *Engine) AttachArgv(cols, rows int) []string {
 	bare := bareAttachArgv(e.Socket(), e.SessionName())
 
@@ -128,11 +135,14 @@ func (e *Engine) AttachArgv(cols, rows int) []string {
 		// because at argv-build time the live window is still the pre-attach size and would be exactly
 		// the wrong answer. The focus target is deliberately discarded: the chain carries select-layout
 		// only, never select-pane.
-		layout, _, err := e.planLayout(st, live, render.Box{X: 0, Y: 0, W: cols, H: rows - reserved})
+		box := render.Box{X: 0, Y: 0, W: cols, H: rows - reserved}
+		layout, _, err := e.planLayout(st, live, box)
 		if err != nil {
 			return err
 		}
 
+		e.installResizePinsLocked(e.fixedHeightPins(st, live, box))
+
 		chained = chainedAttachArgv(e.Socket(), e.SessionName(), layout)
 		return nil
 	})
diff --git a/internal/reedengine/attach_test.go b/internal/reedengine/attach_test.go
index 7580ad782..02290a16c 100644
--- a/internal/reedengine/attach_test.go
+++ b/internal/reedengine/attach_test.go
@@ -41,6 +41,7 @@ type attachRecorder struct {
 	sequence          []string
 	setOptionCalls    [][]string
 	mutationCalls     [][]string
+	setHookCalls      [][]string
 	windowSizeQueried bool
 	liveBoxQueried    bool
 }
@@ -113,6 +114,11 @@ func newAttachHook(script attachScript, rec *attachRecorder) func(capture bool,
 		case "select-layout", "select-pane", "kill-pane", "split-window":
 			rec.mutationCalls = append(rec.mutationCalls, append([]string{}, args...))
 			return "", nil
+		case "set-hook":
+			call := append([]string{}, args...)
+			rec.setHookCalls = append(rec.setHookCalls, call)
+			rec.sequence = append(rec.sequence, "set-hook")
+			return "", nil
 		default:
 			return "", nil
 		}
@@ -388,10 +394,12 @@ func TestAttachArgv_PinsMadeByBuilderBeforeStatusReadback(t *testing.T) {
 	}
 }
 
-// TestAttachArgv_NeverMutatesTheSessionOrPersistsState pins that AttachArgv is read-only end to end:
-// no select-layout, select-pane, kill-pane, or split-window is ever issued (the chain carries
-// select-layout only inside the returned ARGV, never applies it), and reed.json is neither created
-// nor modified by the call.
+// TestAttachArgv_NeverMutatesTheSessionOrPersistsState pins that AttachArgv issues no pane-set
+// mutation: no select-layout, select-pane, kill-pane, or split-window is ever issued (the chain
+// carries select-layout only inside the returned ARGV, never applies it), and reed.json is neither
+// created nor modified by the call. AttachArgv deliberately does mutate a window OPTION now — the
+// resize-pin hook, alongside the two geometry pins it already set — so "never mutates" is scoped to
+// the pane set, not to every tmux call this builder makes.
 func TestAttachArgv_NeverMutatesTheSessionOrPersistsState(t *testing.T) {
 	e, rec := newAttachTestEngine(t, goodAttachScript(), goodAttachStrands())
 
@@ -416,3 +424,121 @@ func TestAttachArgv_NeverMutatesTheSessionOrPersistsState(t *testing.T) {
 		t.Errorf("reed.json changed across AttachArgv: before=%+v after=%+v", before, after)
 	}
 }
+
+// TestAttachArgv_InstallsResizePinsAfterStateAndPanesRead pins the install statement's position in
+// AttachArgv's pre-flight: a known-good pre-flight issues the set-hook clear (and pin rebuild) after
+// the state and pane list are read, and before the argv is returned.
+func TestAttachArgv_InstallsResizePinsAfterStateAndPanesRead(t *testing.T) {
+	e, rec := newAttachTestEngine(t, goodAttachScript(), goodAttachStrands())
+
+	got := e.AttachArgv(80, 24)
+	if len(got) != 10 {
+		t.Fatalf("AttachArgv() = %v, want the 10-element chained argv on this known-good script", got)
+	}
+
+	listPanesIdx, firstSetHookIdx := -1, -1
+	for i, step := range rec.sequence {
+		if step == "list-panes" && listPanesIdx == -1 {
+			listPanesIdx = i
+		}
+		if step == "set-hook" && firstSetHookIdx == -1 {
+			firstSetHookIdx = i
+		}
+	}
+	if listPanesIdx == -1 {
+		t.Fatalf("sequence = %v, want a list-panes call", rec.sequence)
+	}
+	if firstSetHookIdx == -1 {
+		t.Fatalf("sequence = %v, want at least one set-hook call", rec.sequence)
+	}
+	if firstSetHookIdx <= listPanesIdx {
+		t.Errorf("sequence = %v, want the first set-hook call (index %d) after list-panes (index %d)", rec.sequence, firstSetHookIdx, listPanesIdx)
+	}
+	if len(rec.setHookCalls) == 0 {
+		t.Fatal("no set-hook calls recorded, want at least the clear")
+	}
+	if !containsArg(rec.setHookCalls[0], "-u") {
+		t.Errorf("first set-hook argv = %v, want the -u clear", rec.setHookCalls[0])
+	}
+}
+
+// TestAttachArgv_DegradedPathsInstallNoResizePinHook pins that every degraded path yielding the bare
+// argv issues no set-hook call at all — the guard-skip disposition
+// install-points-are-two-named-statements-no-guard-moves documents.
+func TestAttachArgv_DegradedPathsInstallNoResizePinHook(t *testing.T) {
+	t.Run("ZeroCols", func(t *testing.T) {
+		e, rec := newAttachTestEngine(t, goodAttachScript(), goodAttachStrands())
+		assertBareArgv(t, e, e.AttachArgv(0, 24))
+		if len(rec.setHookCalls) != 0 {
+			t.Errorf("set-hook calls = %v, want none", rec.setHookCalls)
+		}
+	})
+	t.Run("HasSessionFails", func(t *testing.T) {
+		script := goodAttachScript()
+		script.hasSessionErr = errors.New("boom")
+		e, rec := newAttachTestEngine(t, script, goodAttachStrands())
+		assertBareArgv(t, e, e.AttachArgv(80, 24))
+		if len(rec.setHookCalls) != 0 {
+			t.Errorf("set-hook calls = %v, want none", rec.setHookCalls)
+		}
+	})
+	t.Run("FewerThanTwoLivePanes", func(t *testing.T) {
+		script := goodAttachScript()
+		script.listPanes = "%1 0 0 40 20 4321\n"
+		e, rec := newAttachTestEngine(t, script, goodAttachStrands())
+		assertBareArgv(t, e, e.AttachArgv(80, 24))
+		if len(rec.setHookCalls) != 0 {
+			t.Errorf("set-hook calls = %v, want none", rec.setHookCalls)
+		}
+	})
+	t.Run("NoStrandOwnsAPresentPane", func(t *testing.T) {
+		e, rec := newAttachTestEngine(t, goodAttachScript(), nil)
+		assertBareArgv(t, e, e.AttachArgv(80, 24))
+		if len(rec.setHookCalls) != 0 {
+			t.Errorf("set-hook calls = %v, want none", rec.setHookCalls)
+		}
+	})
+	t.Run("PlanError_DeferredAnchorRejected", func(t *testing.T) {
+		strands := []Strand{{GUID: "a", PaneID: "%1", Display: render.Display{Anchor: render.AnchorOwnWindow}}}
+		e, rec := newAttachTestEngine(t, goodAttachScript(), strands)
+		assertBareArgv(t, e, e.AttachArgv(80, 24))
+		if len(rec.setHookCalls) != 0 {
+			t.Errorf("set-hook calls = %v, want none", rec.setHookCalls)
+		}
+	})
+}
+
+// TestAttachArgv_SetHookErrorDoesNotChangeTheChainedArgv pins hook-failure-is-non-fatal-everywhere on
+// the AttachArgv path: a set-hook returning an error neither suppresses the chain nor changes a
+// single element of the ten-element chained argv, compared element by element against the same argv
+// built with a non-failing hook.
+func TestAttachArgv_SetHookErrorDoesNotChangeTheChainedArgv(t *testing.T) {
+	e, rec := newAttachTestEngine(t, goodAttachScript(), goodAttachStrands())
+	baseHook := e.tmux.execHook
+
+	want := e.AttachArgv(80, 24)
+	if len(want) != 10 {
+		t.Fatalf("baseline AttachArgv() = %v, want the 10-element chained argv", want)
+	}
+
+	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
+		if args[0] == "set-hook" {
+			out, _ := baseHook(capture, args...)
+			return out, errors.New("boom")
+		}
+		return baseHook(capture, args...)
+	}
+
+	got := e.AttachArgv(80, 24)
+	if len(got) != len(want) {
+		t.Fatalf("AttachArgv() with failing set-hook = %v (len %d), want %v (len %d)", got, len(got), want, len(want))
+	}
+	for i := range want {
+		if got[i] != want[i] {
+			t.Errorf("AttachArgv()[%d] = %q, want %q (a failing set-hook must not change the chained argv)", i, got[i], want[i])
+		}
+	}
+	if len(rec.setHookCalls) == 0 {
+		t.Fatal("no set-hook calls recorded despite the failing hook, want the install statement still attempted")
+	}
+}
diff --git a/internal/reedengine/attachgeometry_integration_test.go b/internal/reedengine/attachgeometry_integration_test.go
index a40615b3c..a3d5805a0 100644
--- a/internal/reedengine/attachgeometry_integration_test.go
+++ b/internal/reedengine/attachgeometry_integration_test.go
@@ -6,6 +6,16 @@
 // passes after (the chained select-layout runs post-attach, once the window already matches the
 // client, so the string lands unchanged).
 //
+// Every case before this task's growth cases below drives a 100x30 client against the fixture's
+// 220x50 boot box — SHORTER than the boot box in both dimensions — so every one of them exercises a
+// window SHRINK at attach time and never the growth path this task is about. And the claim that the
+// chained select-layout running post-attach is what holds the header and collapsed-strip budgets is
+// incomplete on its own: it lands the layout string verbatim only at attach time. tmux has no
+// fixed-height pane concept and redistributes every later window-size delta evenly across the
+// vertical cells, so it is the window-resized resize-pin hook (windowsize.go), not the chain, that
+// holds the budgets across any resize that happens afterward — the growth cases below pin that
+// directly.
+//
 // Linux-only: the pty harness below is built directly on golang.org/x/sys/unix's /dev/ptmx ioctls
 // (TIOCSPTLCK, TIOCGPTN, TIOCSWINSZ), which have no portable equivalent, and psmux's behaviour under
 // a real pty is unverified anywhere in this repo — this file must not even attempt to compile off
@@ -175,12 +185,15 @@ func windowLayoutNow(t *testing.T, e *Engine) string {
 }
 
 // TestAttachGeometry_ExactLayoutAndRowBudgets is the assertion that fails before this task: with a
-// pty deliberately unequal to the configured boot size, it drives a real attach-session through the
-// chained argv AttachArgv builds and asserts, from OUTSIDE the pty, that the live window becomes
-// exactly the client's told size and that #{window_layout} equals the argv's own planned string byte
-// for byte — tmux's silent proportional rescale is what this pins against. It then asserts, from the
-// same attached session, that the header pane and the collapsed strip both landed at their configured,
-// unclamped row budgets.
+// pty deliberately unequal to the configured boot size — a 100x30 client, SHORTER in both dimensions
+// than the fixture's 220x50 boot box, so this is a window SHRINK at attach time, never the growth
+// path the cases below cover — it drives a real attach-session through the chained argv AttachArgv
+// builds and asserts, from OUTSIDE the pty, that the live window becomes exactly the client's told
+// size and that #{window_layout} equals the argv's own planned string byte for byte — tmux's silent
+// proportional rescale is what this pins against. It then asserts, from the same attached session,
+// that the header pane and the collapsed strip both landed at their configured, unclamped row
+// budgets — true here only at attach time; see the growth cases below for what holds those budgets
+// across a later resize.
 func TestAttachGeometry_ExactLayoutAndRowBudgets(t *testing.T) {
 	e := setupAttachGeometryFixture(t)
 
@@ -330,3 +343,191 @@ func TestAttachGeometry_StaleLayoutRaceIsSafe(t *testing.T) {
 		t.Errorf("listPanes after the stale-race attach = %d pane(s), want %d (unchanged from just before the attach)", len(afterLive), len(live))
 	}
 }
+
+// assertAttachGeometryRowBudgets asserts headerPaneID and parentPaneID are, respectively, at
+// e.cfg.Header.HeightRows and e.cfg.CollapsedStripRows in the live pane set, failing with step
+// prefixed onto every message so a caller checking the same budgets at two points in one test can
+// tell which point failed.
+func assertAttachGeometryRowBudgets(t *testing.T, e *Engine, headerPaneID, parentPaneID, step string) {
+	t.Helper()
+	live, err := e.tmux.listPanes(e.SessionName())
+	if err != nil {
+		t.Fatalf("listPanes (%s): %v", step, err)
+	}
+	var sawHeader, sawParent bool
+	for _, p := range live {
+		switch p.ID {
+		case headerPaneID:
+			sawHeader = true
+			if p.Height != e.cfg.Header.HeightRows {
+				t.Errorf("(%s) header pane %s height = %d, want %d (cfg.Header.HeightRows)", step, p.ID, p.Height, e.cfg.Header.HeightRows)
+			}
+		case parentPaneID:
+			sawParent = true
+			if p.Height != e.cfg.CollapsedStripRows {
+				t.Errorf("(%s) collapsed parent pane %s height = %d, want %d (cfg.CollapsedStripRows)", step, p.ID, p.Height, e.cfg.CollapsedStripRows)
+			}
+		}
+	}
+	if !sawHeader {
+		t.Fatalf("(%s) header pane %s missing from live panes %+v", step, headerPaneID, live)
+	}
+	if !sawParent {
+		t.Fatalf("(%s) collapsed parent pane %s missing from live panes %+v", step, parentPaneID, live)
+	}
+}
+
+// TestAttachGeometry_ResizeAfterAttachHoldsRowBudgets pins the fix this task ships: unlike
+// TestAttachGeometry_ExactLayoutAndRowBudgets above, whose 100x30 client is SHORTER than the fixture's
+// 220x50 boot box and so never exercises anything beyond attach time, this case resizes the pty AFTER
+// a healthy chained attach has already landed the header and collapsed-strip budgets, to a materially
+// TALLER size, and asserts both budgets still hold. This is the case that fails before this task: tmux
+// has no fixed-height pane concept and redistributes every window-size delta evenly across the
+// vertical cells with no intervention, and it is the window-resized resize-pin hook installed by this
+// task — not the chained select-layout, which only ever runs once, at attach time — that holds the
+// budgets across the resize.
+func TestAttachGeometry_ResizeAfterAttachHoldsRowBudgets(t *testing.T) {
+	e := setupAttachGeometryFixture(t)
+
+	const cols, rows = 100, 30
+	argv := e.AttachArgv(cols, rows)
+	if len(argv) != 10 {
+		t.Fatalf("AttachArgv(%d, %d) = %v (%d argv elements), want the 10-element chained form", cols, rows, argv, len(argv))
+	}
+
+	pty := startInPTY(t, append([]string{e.cfg.Tmux}, argv...), cols, rows)
+	waitForClientAttached(t, e, 15*time.Second)
+
+	st, err := LoadState(e.stateDir())
+	if err != nil || st == nil {
+		t.Fatalf("LoadState = (%+v, %v), want a readable state", st, err)
+	}
+	if len(st.Strands) != 2 {
+		t.Fatalf("st.Strands = %+v, want exactly 2 (the shrink-when-waiting parent and its child)", st.Strands)
+	}
+	parentPaneID := st.Strands[0].PaneID
+	headerPaneID := st.HeaderPaneID
+
+	// Confirm the budgets landed at attach time, exactly as the exact-layout case above pins.
+	assertAttachGeometryRowBudgets(t, e, headerPaneID, parentPaneID, "after attach")
+
+	// Now drive a real client resize on the pty master — materially TALLER than the attach size, and
+	// taller than the 220x50 boot box's 50 rows too, so this is unambiguously the growth path, never
+	// a second shrink.
+	const resizedCols, resizedRows = 100, 90
+	if err := unix.IoctlSetWinsize(int(pty.master.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Col: uint16(resizedCols), Row: uint16(resizedRows)}); err != nil {
+		t.Fatalf("TIOCSWINSZ (%dx%d): %v", resizedCols, resizedRows, err)
+	}
+	waitUntil(t, 15*time.Second, "window never reported the resized height", func() bool {
+		_, h := windowSizeNow(t, e)
+		return h == resizedRows
+	})
+
+	assertAttachGeometryRowBudgets(t, e, headerPaneID, parentPaneID, "after resize")
+}
+
+// TestAttachGeometry_BareAttachFromTallClientStillHoldsHeaderBudget covers the path the originally
+// reported ~50-row threshold came from: a bare, unchained attach (AttachArgv(0, 0), the "no client
+// size known" argv) from a client TALLER than the fixture's 220x50 boot box's 50-row height. It
+// asserts the header pane still settles at e.cfg.Header.HeightRows once the attach settles.
+//
+// This case does NOT exercise an install of its own: AttachArgv(0, 0) returns the bare argv before
+// the op lock is even taken, so it installs no window-resized hook. What holds the header here is the
+// hook setupAttachGeometryFixture's own earlier AddStrand calls already installed via
+// applyLayoutLocked — this case passes on that pre-existing hook, not on anything AttachArgv(0, 0)
+// itself does, and must not be misread as proof that the no-client-size path installs one.
+func TestAttachGeometry_BareAttachFromTallClientStillHoldsHeaderBudget(t *testing.T) {
+	e := setupAttachGeometryFixture(t)
+
+	argv := e.AttachArgv(0, 0)
+	if len(argv) != 5 {
+		t.Fatalf("AttachArgv(0, 0) = %v (%d argv elements), want the 5-element bare form with no chained select-layout", argv, len(argv))
+	}
+
+	st, err := LoadState(e.stateDir())
+	if err != nil || st == nil {
+		t.Fatalf("LoadState = (%+v, %v), want a readable state", st, err)
+	}
+	headerPaneID := st.HeaderPaneID
+
+	const cols, rows = 100, 80 // taller than the 220x50 boot box's 50 rows
+	startInPTY(t, append([]string{e.cfg.Tmux}, argv...), cols, rows)
+	waitForClientAttached(t, e, 15*time.Second)
+
+	waitUntil(t, 15*time.Second, "header pane never settled at cfg.Header.HeightRows after the bare attach", func() bool {
+		live, err := e.tmux.listPanes(e.SessionName())
+		if err != nil {
+			return false
+		}
+		for _, p := range live {
+			if p.ID == headerPaneID {
+				return p.Height == e.cfg.Header.HeightRows
+			}
+		}
+		return false
+	})
+}
+
+// TestAttachGeometry_DeadStripPinDoesNotBreakHeaderPin pins the fire-time failure isolation the
+// window-resized hook's array encoding buys (Shared Decision hook-body-is-one-array-entry-per-pin):
+// after a healthy chained attach installs a hook pinning both the header and the collapsed parent
+// strip, this kills the strip's own pane out from under the hook, leaving its array entry naming a
+// destroyed pane id, then resizes and asserts the header still holds its budget — the header is
+// always pin index 0, and independent array entries mean a later entry's failure cannot take an
+// earlier one down with it (contract_integration_test.go's TestMultiplexerContract pins the same wire
+// fact directly, at the set-hook level, with no pty involved).
+func TestAttachGeometry_DeadStripPinDoesNotBreakHeaderPin(t *testing.T) {
+	e := setupAttachGeometryFixture(t)
+
+	const cols, rows = 100, 30
+	argv := e.AttachArgv(cols, rows)
+	if len(argv) != 10 {
+		t.Fatalf("AttachArgv(%d, %d) = %v (%d argv elements), want the 10-element chained form", cols, rows, argv, len(argv))
+	}
+
+	pty := startInPTY(t, append([]string{e.cfg.Tmux}, argv...), cols, rows)
+	waitForClientAttached(t, e, 15*time.Second)
+
+	st, err := LoadState(e.stateDir())
+	if err != nil || st == nil {
+		t.Fatalf("LoadState = (%+v, %v), want a readable state", st, err)
+	}
+	if len(st.Strands) != 2 {
+		t.Fatalf("st.Strands = %+v, want exactly 2 (the shrink-when-waiting parent and its child)", st.Strands)
+	}
+	headerPaneID := st.HeaderPaneID
+	parentPaneID := st.Strands[0].PaneID
+
+	// Kill the collapsed strip's own pane directly via e.tmux, bypassing RemoveStrand/reconcile
+	// entirely: the installed hook's strip-pin entry now names a destroyed pane id, exactly the state
+	// a stray operator kill or a crashed strand process would leave behind.
+	if err := e.tmux.run("kill-pane", "-t", parentPaneID); err != nil {
+		t.Fatalf("kill-pane -t %s: %v", parentPaneID, err)
+	}
+
+	const resizedCols, resizedRows = 100, 90
+	if err := unix.IoctlSetWinsize(int(pty.master.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Col: uint16(resizedCols), Row: uint16(resizedRows)}); err != nil {
+		t.Fatalf("TIOCSWINSZ (%dx%d): %v", resizedCols, resizedRows, err)
+	}
+	waitUntil(t, 15*time.Second, "window never reported the resized height", func() bool {
+		_, h := windowSizeNow(t, e)
+		return h == resizedRows
+	})
+
+	live, err := e.tmux.listPanes(e.SessionName())
+	if err != nil {
+		t.Fatalf("listPanes: %v", err)
+	}
+	var sawHeader bool
+	for _, p := range live {
+		if p.ID == headerPaneID {
+			sawHeader = true
+			if p.Height != e.cfg.Header.HeightRows {
+				t.Errorf("header pane %s height = %d, want %d (cfg.Header.HeightRows) even though the strip pin's own pane was destroyed", p.ID, p.Height, e.cfg.Header.HeightRows)
+			}
+		}
+	}
+	if !sawHeader {
+		t.Fatalf("header pane %s missing from live panes %+v", headerPaneID, live)
+	}
+}
diff --git a/internal/reedengine/contract_integration_test.go b/internal/reedengine/contract_integration_test.go
index c66e55bf4..58bd5ab41 100644
--- a/internal/reedengine/contract_integration_test.go
+++ b/internal/reedengine/contract_integration_test.go
@@ -224,6 +224,93 @@ func TestMultiplexerContract(t *testing.T) {
 		t.Fatalf("select-pane: %v", err)
 	}
 
+	// set-hook / resize-pane: reed's first deliberately OPTIONAL wire surface — absent from
+	// requiredSubcommands on purpose (doc.go's "Subcommand set" paragraph), so this section
+	// documents their semantics rather than gating the capability probe on them. Both wire
+	// behaviours the resize-pin hook (windowsize.go) rests on, and that no unit test can reach,
+	// are pinned here: array independence across set-hook -u / set-hook / set-hook -a, and a
+	// window-resized hook firing AFTER tmux has already resized the layout.
+	windowTarget := exactSessionWindowTarget(session)
+
+	// First: set-hook -u followed by set-hook and one set-hook -a yields INDEPENDENT array
+	// entries. Entry [0] deliberately names a pane id no session on this socket owns, so the
+	// live-firing half below can assert its failure does not take entry [1] down with it.
+	if err := reed.run("set-hook", "-u", "-w", "-t", windowTarget, "window-resized"); err != nil {
+		t.Fatalf("set-hook -u -w -t %q window-resized: %v", windowTarget, err)
+	}
+	if err := reed.run("set-hook", "-w", "-t", windowTarget, "window-resized", "resize-pane -t %99 -y 1"); err != nil {
+		t.Fatalf("set-hook -w -t %q window-resized (entry [0]): %v", windowTarget, err)
+	}
+	if err := reed.run("set-hook", "-a", "-w", "-t", windowTarget, "window-resized", fmt.Sprintf("resize-pane -t %s -y 1", initialPane.ID)); err != nil {
+		t.Fatalf("set-hook -a -w -t %q window-resized (entry [1]): %v", windowTarget, err)
+	}
+
+	// Read the array back: one line per entry, each naming its own pane id — the readback shape
+	// verified live on tmux 3.6.
+	hooksOut, err := reed.output("show-hooks", "-w", "-t", windowTarget)
+	if err != nil {
+		t.Fatalf("show-hooks -w -t %q: %v", windowTarget, err)
+	}
+	if !strings.Contains(hooksOut, "window-resized[0]") || !strings.Contains(hooksOut, "%99") {
+		t.Errorf("show-hooks -w -t %q = %q, want a window-resized[0] line naming pane %%99", windowTarget, hooksOut)
+	}
+	if !strings.Contains(hooksOut, "window-resized[1]") || !strings.Contains(hooksOut, initialPane.ID) {
+		t.Errorf("show-hooks -w -t %q = %q, want a window-resized[1] line naming pane %s", windowTarget, hooksOut, initialPane.ID)
+	}
+
+	// Second: a window-resized hook fires AFTER tmux has resized the layout, so a resize-pane -y
+	// inside it survives the resize that triggered it — and the dead entry [0] above does not
+	// take entry [1] down with it. window-size manual is required to make a DETACHED session
+	// resizable at all.
+	if err := reed.run("set-option", "-w", "-t", windowTarget, "window-size", "manual"); err != nil {
+		t.Fatalf("set-option -w -t %q window-size manual: %v", windowTarget, err)
+	}
+	if err := reed.run("resize-window", "-t", windowTarget, "-x", "80", "-y", "60"); err != nil {
+		t.Fatalf("resize-window -t %q -x 80 -y 60: %v", windowTarget, err)
+	}
+	waitUntil(t, 10*time.Second, "listPanes never reported the window fully laid out at 60 rows", func() bool {
+		live, err := reed.listPanes(session)
+		if err != nil {
+			return false
+		}
+		maxBottom := 0
+		for _, p := range live {
+			if bottom := p.Top + p.Height; bottom > maxBottom {
+				maxBottom = bottom
+			}
+		}
+		return maxBottom == 60
+	})
+	afterResizeLive, err := reed.listPanes(session)
+	if err != nil {
+		t.Fatalf("listPanes after resize-window: %v", err)
+	}
+	var pinnedHeight int
+	var pinnedFound bool
+	for _, p := range afterResizeLive {
+		if p.ID == initialPane.ID {
+			pinnedHeight = p.Height
+			pinnedFound = true
+		}
+	}
+	if !pinnedFound {
+		t.Fatalf("listPanes after resize-window = %+v, want pane %s still present", afterResizeLive, initialPane.ID)
+	}
+	if pinnedHeight != 1 {
+		t.Errorf("pane %s height after resize-window = %d, want exactly 1 (the resize-pane -y entry [1] fired after tmux's own resize and pinned it)", initialPane.ID, pinnedHeight)
+	}
+
+	// Leave the window as the later steps of this test expect to find it, using two named
+	// mechanisms and no ad-hoc readback-and-restore: clear the hook array, and drop the
+	// window-size override with the -u unset form rather than reading the prior value back and
+	// re-setting it.
+	if err := reed.run("set-hook", "-u", "-w", "-t", windowTarget, "window-resized"); err != nil {
+		t.Fatalf("set-hook -u -w -t %q window-resized (final clear): %v", windowTarget, err)
+	}
+	if err := reed.run("set-option", "-uw", "-t", windowTarget, "window-size"); err != nil {
+		t.Fatalf("set-option -uw -t %q window-size (unset): %v", windowTarget, err)
+	}
+
 	// (b) list-sessions: the subcommand serverPIDLocked's sibling reap
 	// helpers use to distinguish "no server" from "server up".
 	sessionsOut, err := reed.output("list-sessions", "-F", "#{session_name}")
diff --git a/internal/reedengine/doc.go b/internal/reedengine/doc.go
index 1c2144885..07862ac6b 100644
--- a/internal/reedengine/doc.go
+++ b/internal/reedengine/doc.go
@@ -45,14 +45,32 @@
 // go. It boots alongside the session/initial pane on both Up and Resume, and
 // Engine.ValidateHeader runs eagerly on every boot path so a bad header
 // template surfaces loud before the pane is ever created, never silently.
-// A header whose keepalive process dies (pane_dead=1) is deliberately kept
-// as an enumerable corpse by reconcile — never killed there — and healed
-// (corpse killed, a fresh header split back in at the physical top) by
-// ensureHeaderPaneLocked on the next Up/Resume; planLayout only ever emits
-// a header cell for a pane actually present in the window, so a stale
-// HeaderPaneID can never put an absent pane's cell into select-layout's
-// string (which a real tmux accepts and misassigns positionally rather
-// than rejecting).
+// The header pane is created by a split-window call that carries the
+// keepalive command (`lyx reed header --blocking`) as its own trailing
+// shell-command argument, rather than by splitting a bare shell and typing
+// the command into it afterwards with send-keys: the pane runs that command
+// directly from birth, so it hosts no interactive shell for anything to echo
+// the launch line into or read ~/.bashrc from. This makes the corpse-and-heal
+// contract below actually work as documented: with the keepalive as the
+// pane's own process, "set-option -g remain-on-exit on" corpses the pane the
+// moment that process dies, where a surviving bash previously kept the pane
+// alive and a dead header was silently mistaken for a working one. Under
+// go test the pane still boots commandless — a bare shell, no split-window
+// trailing argument and no send-keys — because headerLaunchLine
+// (headerpane.go) returns "" whenever the boot decides to suppress the
+// launch, which prevents os.Executable() from re-exec'ing the test binary
+// and running its whole suite recursively; that decision now rides on
+// Engine.suppressHeaderLaunch, an unexported field New initialises from
+// testing.Testing(), rather than a testing.Testing() call hard-wired at the
+// boot site. A header whose keepalive process dies (pane_dead=1) is
+// deliberately kept as an enumerable corpse by reconcile — never killed
+// there — and healed (corpse killed, a fresh header split back in at the
+// physical top, carrying the same launch command on both the first attempt
+// and any even-vertical-retile retry) by ensureHeaderPaneLocked on the next
+// Up/Resume; planLayout only ever emits a header cell for a pane actually
+// present in the window, so a stale HeaderPaneID can never put an absent
+// pane's cell into select-layout's string (which a real tmux accepts and
+// misassigns positionally rather than rejecting).
 //
 // The live-geometry rule: the render box a layout is computed against is no
 // longer the config-pinned Width/Height. planLayout (apply.go) is always
@@ -121,6 +139,16 @@
 // therefore issued through TmuxCmd, carrying reed's own -L socket, so the
 // probe can never start a server on the operator's GLOBAL DEFAULT socket
 // — see probeCapabilityLocked for what that cost while it did.
+// Every verb named above is REQUIRED: a binary missing any of them is
+// unusable, and requiredSubcommands (probe.go) fails the capability probe
+// on it.
+// set-hook and resize-pane are different: they are reed's first
+// deliberately OPTIONAL verbs, absent from requiredSubcommands on purpose,
+// because gating the capability probe on them would take every reed verb
+// down on a psmux lacking set-hook, over a quality-only option
+// (the resize-pin hook documented below, under "The resize round-robin
+// and the resize-pin hook") that is already designed to degrade silently
+// — their absence costs only the resize pin, never a working session.
 //
 // Load-bearing behavioral assumptions, each with the rationale that makes it
 // load-bearing:
@@ -309,13 +337,54 @@
 //     configured boot height until the next attach snaps it back; with a
 //     client attached, the "window-size latest" pin holds the window at the
 //     client's size and tmux rescales the cells into it instead.
+//   - The resize round-robin and the resize-pin hook (windowsize.go,
+//     apply.go, attach.go): tmux has no fixed-height pane concept, and a
+//     window-size delta arriving after attach time is handed out one row at
+//     a time, round-robin across every vertical cell in the stack — so no
+//     absolute row budget reed computes survives a resize on its own.
+//     Measured live on tmux 3.6: a healthy attached session's header went
+//     from 1 row to 6 across a 76-to-90-row client resize, and to 16 across
+//     a further 90-to-120 one.
+//     The answer is a window-resized window hook holding one
+//     "resize-pane -y" array entry per fixed-height pane — the header band
+//     and every collapsed strip — installed by reed and executed by the
+//     tmux server itself, refreshed on every successful apply
+//     (applyLayoutLocked) and again in AttachArgv's pre-flight, with the
+//     pinned heights coming from render.FixedHeightPins: the heights render
+//     actually placed the cells at, after clampHeaderHeight and
+//     clampToFit, never the raw configured budgets.
+//     Two candidate hooks were measured and rejected: client-resized fires
+//     BEFORE the layout is resized, so a resize-pane inside it cannot work,
+//     and window-layout-changed also fires on reed's own select-layout,
+//     inviting re-entrancy for no benefit.
+//     The paths that install nothing are deliberate: an apply returning at
+//     either of applyLayoutLocked's guards, and every AttachArgv degrade
+//     return, issue no set-hook at all — not even the clear — so a
+//     previously installed array survives them on purpose, since a clear
+//     with no rebuild behind it would drift on the very next resize.
+//     That is safe in both guard cases. resize-pane -y against a window's
+//     sole pane is a verified silent no-op (exit 0, height unchanged), so
+//     the len(live) < 2 case's surviving header pin cannot contradict
+//     render.Rules' sole-cell branch.
+//     And in the !anyPlacedStrand case — reachable for good via the
+//     operator remedy state.go documents, which deletes reed.json while the
+//     session keeps running untracked — the surviving array is a benefit,
+//     still holding the live header and strips at the budgets reed last
+//     computed for them.
+//     The ~50-row threshold in the original bug report is
+//     template_posix.yaml's "height: 50" boot box showing through the BARE
+//     (unchained) attach path, not evidence of a miscomputed layout — a
+//     synthetic bare attach reproduces the reported table exactly, with 40
+//     and 50 rows leaving the header at 1 row and 76 rows taking it to 10.
 //   - The chained attach (attach.go): AttachArgv's argv is
 //     "attach-session … ; select-layout -t '=<session>:' <layout>", with the
 //     separator a literal one-character ";" argv element — never "\;",
 //     since exec.Command passes argv directly to the child and no shell ever
 //     sees it to unescape. The chained select-layout runs only after the
 //     client has attached and tmux has already resized the window to it, so
-//     the layout string lands verbatim with no rescale. attach-session is
+//     the layout string lands verbatim with no rescale — but only until the
+//     next window resize; see the resize round-robin bullet above for what
+//     holds the fixed-height budgets afterward. attach-session is
 //     first in the chain, so a failing or unsupported select-layout still
 //     leaves the operator attached — strictly no worse than before this
 //     task. The window between building the string and applying it is not
@@ -395,8 +464,14 @@
 //     booting a header pane, which is why the tier-2 proof runs the loop
 //     in-process against a real session instead.
 //
-// requiredSubcommands (probe.go) did not grow for any of this: display-message,
-// select-layout, set-option, and list-panes were already spent by the engine
-// before this task, so the live-geometry rule, the attach chain, and the two
-// option pins add no capability-probe change and no new psmux risk.
+// requiredSubcommands (probe.go) still does not grow for the live-geometry
+// rule, the attach chain, or the two option pins: display-message,
+// select-layout, set-option, and list-panes were already spent by the
+// engine before this task.
+// set-hook and resize-pane are not the same story: both are new to
+// internal/, so the wire contract this package assumes genuinely widens,
+// and that widening costing nothing at the probe is now a deliberate
+// trade, not a free consequence — the non-fatal degrade the two Optional
+// verbs above are wired for (Shared Decision
+// hook-failure-is-non-fatal-everywhere) is what pays for it.
 package reedengine
diff --git a/internal/reedengine/lifecycle.go b/internal/reedengine/lifecycle.go
index bb4b3e026..33f6003a8 100644
--- a/internal/reedengine/lifecycle.go
+++ b/internal/reedengine/lifecycle.go
@@ -15,7 +15,6 @@ import (
 	"path/filepath"
 	"strconv"
 	"strings"
-	"testing"
 	"time"
 
 	"github.com/Knatte18/loomyard/internal/logger"
@@ -504,7 +503,20 @@ func (e *Engine) ensureHeaderPaneLocked(st *ReedState) error {
 		return fmt.Errorf("resolve lyx binary path: %w", err)
 	}
 
-	paneID, err := e.splitHeaderPaneAtTopLocked(session, live)
+	// Computed above the split so it can be passed straight into split-window as its own trailing
+	// shell-command argument: the pane then runs launchCmd directly from birth, hosting no
+	// interactive shell for anything to echo the command into or read ~/.bashrc from.
+	launchCmd := headerLaunchLine(shell.ForGOOS(), exe, e.suppressHeaderLaunch)
+	if launchCmd == "" {
+		// Under go test the header pane stays a bare blocking shell — see
+		// headerLaunchLine: re-exec'ing exe here would run the test binary's
+		// entire suite recursively. The pane still exists and its id is still
+		// recorded below, so layout geometry and up/resume idempotence are
+		// unchanged.
+		logger.Info("reed: header re-exec suppressed under go test, pane left as bare shell", "socket", e.Socket(), "exe", exe)
+	}
+
+	paneID, err := e.splitHeaderPaneAtTopLocked(session, live, launchCmd)
 	if err != nil {
 		return fmt.Errorf("split header pane: %w", err)
 	}
@@ -520,28 +532,6 @@ func (e *Engine) ensureHeaderPaneLocked(st *ReedState) error {
 		}
 	}
 
-	launchCmd := headerLaunchLine(shell.ForGOOS(), exe, testing.Testing())
-	if launchCmd == "" {
-		// Under go test the header pane stays a bare blocking shell — see
-		// headerLaunchLine: re-exec'ing exe here would run the test binary's
-		// entire suite recursively. The pane still exists and its id is still
-		// recorded below, so layout geometry and up/resume idempotence are
-		// unchanged.
-		logger.Info("reed: header re-exec suppressed under go test, pane left as bare shell", "socket", e.Socket(), "pane", paneID, "exe", exe)
-	} else {
-		// Same literal send-keys mechanics launchStrandLocked (spawn.go) uses:
-		// -l so tmux never reinterprets any part of the launch line, then a
-		// separate Enter to submit it.
-		if err := e.tmux.run("send-keys", "-t", paneID, "-l", sendKeysLiteralArg(launchCmd)); err != nil {
-			logger.Warn("reed: failed to send header launch command", "socket", e.Socket(), "pane", paneID, "err", err)
-			return fmt.Errorf("send header launch command: %w", err)
-		}
-		if err := e.tmux.run("send-keys", "-t", paneID, "Enter"); err != nil {
-			logger.Warn("reed: failed to submit header launch command", "socket", e.Socket(), "pane", paneID, "err", err)
-			return fmt.Errorf("submit header launch command: %w", err)
-		}
-	}
-
 	st.HeaderPaneID = paneID
 	if err := SaveState(e.stateDir(), st); err != nil {
 		return fmt.Errorf("persist header pane id: %w", err)
@@ -588,8 +578,8 @@ func topmostPaneID(live []LivePane) string {
 // On a failed retry the FIRST error is returned, not the retry's: it describes the state the
 // operator actually has, and the re-tile is an internal repair attempt rather than something they
 // asked for.
-func (e *Engine) splitHeaderPaneAtTopLocked(session string, live []LivePane) (string, error) {
-	paneID, firstErr := e.splitPaneAboveLocked(topmostPaneID(live), live)
+func (e *Engine) splitHeaderPaneAtTopLocked(session string, live []LivePane, launchCmd string) (string, error) {
+	paneID, firstErr := e.splitPaneAboveLocked(topmostPaneID(live), live, launchCmd)
 	if firstErr == nil {
 		return paneID, nil
 	}
@@ -604,7 +594,9 @@ func (e *Engine) splitHeaderPaneAtTopLocked(session string, live []LivePane) (st
 		logger.Warn("reed: could not re-enumerate panes after the even-vertical re-tile", "socket", e.Socket(), "session", session, "err", err)
 		return "", firstErr
 	}
-	paneID, err = e.splitPaneAboveLocked(topmostPaneID(retiled), retiled)
+	// The retry carries launchCmd too — a retried header must never boot commandless, or it would
+	// be left hosting an interactive shell exactly like the noise this batch removes.
+	paneID, err = e.splitPaneAboveLocked(topmostPaneID(retiled), retiled, launchCmd)
 	if err != nil {
 		logger.Warn("reed: header split still had no room after the even-vertical re-tile", "socket", e.Socket(), "session", session, "err", err)
 		return "", firstErr
@@ -631,8 +623,17 @@ func (e *Engine) splitHeaderPaneAtTopLocked(session string, live []LivePane) (st
 // header would bind the header to a strand's pane — the next layout string would then carry a
 // duplicate pane number, destroying the session's panes wholesale (see
 // validateSplitCreatedNewPane).
-func (e *Engine) splitPaneAboveLocked(target string, preSplitLive []LivePane) (string, error) {
-	out, err := e.tmux.output("split-window", "-b", "-t", target, "-c", e.geom.PaneCwd, "-P", "-F", "#{pane_id}")
+func (e *Engine) splitPaneAboveLocked(target string, preSplitLive []LivePane, launchCmd string) (string, error) {
+	argv := []string{"split-window", "-b", "-t", target, "-c", e.geom.PaneCwd, "-P", "-F", "#{pane_id}"}
+	if launchCmd != "" {
+		// A single trailing shell-command argument, exactly like an interactive `tmux split-window`
+		// invocation's own trailing-command syntax: the pane then runs launchCmd directly rather than
+		// an interactive shell, so nothing types it, echoes it, or reads a shell rc file for it. Empty
+		// launchCmd leaves the argv exactly as it was before this parameter existed — the go test
+		// default (see Engine.suppressHeaderLaunch).
+		argv = append(argv, launchCmd)
+	}
+	out, err := e.tmux.output(argv...)
 	if err != nil {
 		return "", err
 	}
diff --git a/internal/reedengine/lifecycle_test.go b/internal/reedengine/lifecycle_test.go
index ca4648ecc..c04826bb3 100644
--- a/internal/reedengine/lifecycle_test.go
+++ b/internal/reedengine/lifecycle_test.go
@@ -486,6 +486,190 @@ func TestEnsureHeaderPaneLocked_RecoversWhenTheTopPaneIsTooSmallToSplit(t *testi
 	}
 }
 
+// enableHeaderLaunch flips e.suppressHeaderLaunch back off, undoing the testing.Testing()-derived
+// default New sets. It is the seam P1 needs: nothing outside this package can reach the unexported
+// field, so a test that must drive the real header-launch path against a fake tmux does it through
+// this helper rather than by exporting the field.
+func enableHeaderLaunch(t *testing.T, e *Engine) {
+	t.Helper()
+	e.suppressHeaderLaunch = false
+}
+
+// TestEnsureHeaderPaneLocked_LaunchesTheCommandOnTheSplitNotViaSendKeys is P1: it pins that the
+// header pane is booted by handing split-window the launch line as its own trailing shell-command
+// argument, not by typing it into an interactive shell afterwards via send-keys. Both halves matter
+// — a fix that carries the command on the argv but still sends keys, or vice versa, must fail this.
+//
+// No #{pane_current_command} assertion is added: that value is shell-dependent and this fake-tmux
+// substrate never runs a real shell.
+func TestEnsureHeaderPaneLocked_LaunchesTheCommandOnTheSplitNotViaSendKeys(t *testing.T) {
+	e := newTestEngine(t)
+	enableHeaderLaunch(t, e)
+
+	const existingPaneID = "%0"
+	const newPaneID = "%1"
+	listPanesOut := existingPaneID + " 0 0 100 20 4321\n"
+
+	var splitArgs []string
+	sendKeysCalls := 0
+	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
+		switch args[0] {
+		case "list-panes":
+			return listPanesOut, nil
+		case "split-window":
+			splitArgs = append([]string{}, args...)
+			// A genuinely new pane id, distinct from the pre-split live set, so
+			// the silent-split guard (validateSplitCreatedNewPane) does not
+			// reject the call.
+			return newPaneID + "\n", nil
+		case "send-keys":
+			sendKeysCalls++
+			return "", nil
+		default:
+			return "", nil
+		}
+	}
+
+	st := &ReedState{Socket: e.Socket(), Session: e.SessionName()}
+	if err := e.ensureHeaderPaneLocked(st); err != nil {
+		t.Fatalf("ensureHeaderPaneLocked: %v", err)
+	}
+
+	fIndex := -1
+	for i, arg := range splitArgs {
+		if arg == "-F" {
+			fIndex = i
+			break
+		}
+	}
+	if fIndex == -1 {
+		t.Fatalf("split-window argv %v has no -F flag", splitArgs)
+	}
+	if fIndex+1 >= len(splitArgs) {
+		t.Fatalf("split-window argv %v has a trailing -F with no value", splitArgs)
+	}
+	if fIndex+2 >= len(splitArgs) {
+		t.Fatalf("split-window argv %v carries no trailing command argument after the -F value; want the launch line appended as split-window's own trailing shell-command argument", splitArgs)
+	}
+	launchArg := splitArgs[fIndex+2]
+	// A substring check, not an exact match, because headerLaunchCmd's posix and pwsh quoting differ
+	// — this must hold for both.
+	for _, want := range []string{"reed", "--blocking"} {
+		if !strings.Contains(launchArg, want) {
+			t.Errorf("split-window trailing command argument = %q, want it to contain %q (the header keepalive invocation)", launchArg, want)
+		}
+	}
+	if sendKeysCalls != 0 {
+		t.Errorf("send-keys calls = %d, want 0 (the header pane must launch its own command on the split, not be typed into via send-keys)", sendKeysCalls)
+	}
+}
+
+// TestEnsureHeaderPaneLocked_DefaultUnderGoTestSplitsACommandlessShell pins the preserved go test
+// default: a default newTestEngine leaves suppressHeaderLaunch on (New derives it from
+// testing.Testing()), so the header split must still carry no trailing command argument and issue no
+// send-keys, exactly as it always has — this batch changes how a launch command travels, not whether
+// one is issued under go test.
+func TestEnsureHeaderPaneLocked_DefaultUnderGoTestSplitsACommandlessShell(t *testing.T) {
+	e := newTestEngine(t)
+
+	const existingPaneID = "%0"
+	const newPaneID = "%1"
+	listPanesOut := existingPaneID + " 0 0 100 20 4321\n"
+
+	var splitArgs []string
+	sendKeysCalls := 0
+	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
+		switch args[0] {
+		case "list-panes":
+			return listPanesOut, nil
+		case "split-window":
+			splitArgs = append([]string{}, args...)
+			return newPaneID + "\n", nil
+		case "send-keys":
+			sendKeysCalls++
+			return "", nil
+		default:
+			return "", nil
+		}
+	}
+
+	st := &ReedState{Socket: e.Socket(), Session: e.SessionName()}
+	if err := e.ensureHeaderPaneLocked(st); err != nil {
+		t.Fatalf("ensureHeaderPaneLocked: %v", err)
+	}
+
+	if len(splitArgs) == 0 || splitArgs[len(splitArgs)-1] != "#{pane_id}" {
+		t.Errorf("split-window argv = %v, want it to end at the -F value #{pane_id} with no trailing command argument (go test default: bare-shell header)", splitArgs)
+	}
+	if sendKeysCalls != 0 {
+		t.Errorf("send-keys calls = %d, want 0 under the go test default", sendKeysCalls)
+	}
+	if st.HeaderPaneID != newPaneID {
+		t.Errorf("HeaderPaneID = %q, want %q (recorded even though the pane is commandless)", st.HeaderPaneID, newPaneID)
+	}
+}
+
+// TestEnsureHeaderPaneLocked_RetriedSplitAlsoCarriesTheLaunchCommand reuses
+// TestEnsureHeaderPaneLocked_RecoversWhenTheTopPaneIsTooSmallToSplit's wedged/retiled scripted
+// substrate (a one-row top pane the first split-window refuses, an even-vertical re-tile, then a
+// successful retry) but with launch enabled, and pins that the RETRIED split-window call — not just
+// a hypothetical first one — carries the launch command too. A retry path that dropped launchCmd
+// would boot a recovered header as an interactive shell, silently reopening this batch's noise class
+// on exactly the wedged-worktree recovery path R4-F4 exists for.
+func TestEnsureHeaderPaneLocked_RetriedSplitAlsoCarriesTheLaunchCommand(t *testing.T) {
+	e := newTestEngine(t)
+	enableHeaderLaunch(t, e)
+
+	const oneRowTopPaneID = "%1"
+	const tallPaneID = "%0"
+	const rebuiltHeaderPaneID = "%7"
+	// pane_id pane_dead pane_top pane_width pane_height pane_pid
+	wedged := oneRowTopPaneID + " 0 0 100 1 4321\n" + tallPaneID + " 0 2 100 48 4322\n"
+	retiled := oneRowTopPaneID + " 0 0 100 25 4321\n" + tallPaneID + " 0 26 100 24 4322\n"
+
+	reTiled := false
+	var retriedSplitArgs []string
+	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
+		switch args[0] {
+		case "list-panes":
+			if reTiled {
+				return retiled, nil
+			}
+			return wedged, nil
+		case "select-layout":
+			reTiled = true
+			return "", nil
+		case "split-window":
+			if !reTiled {
+				// tmux's real refusal against a one-row pane: exit 1, no pane.
+				return "", errors.New("exit status 1: no space for new pane")
+			}
+			retriedSplitArgs = append([]string{}, args...)
+			return rebuiltHeaderPaneID + "\n", nil
+		default:
+			return "", nil
+		}
+	}
+
+	st := &ReedState{Socket: e.Socket(), Session: e.SessionName()}
+	if err := e.ensureHeaderPaneLocked(st); err != nil {
+		t.Fatalf("ensureHeaderPaneLocked() = %v; want nil", err)
+	}
+	if st.HeaderPaneID != rebuiltHeaderPaneID {
+		t.Fatalf("HeaderPaneID = %q; want %q (the pane the retried split created)", st.HeaderPaneID, rebuiltHeaderPaneID)
+	}
+
+	if len(retriedSplitArgs) == 0 {
+		t.Fatalf("the retried split-window call was never recorded")
+	}
+	launchArg := retriedSplitArgs[len(retriedSplitArgs)-1]
+	for _, want := range []string{"reed", "--blocking"} {
+		if !strings.Contains(launchArg, want) {
+			t.Errorf("retried split-window trailing argument = %q, want it to contain %q (a retried header must never boot commandless)", launchArg, want)
+		}
+	}
+}
+
 // TestTopmostPaneID asserts the header split target is chosen by pane_top rather than by list-panes
 // order, which tmux does not guarantee is top-to-bottom.
 func TestTopmostPaneID(t *testing.T) {
diff --git a/internal/reedengine/lock.go b/internal/reedengine/lock.go
index e12ead348..48135d0a6 100644
--- a/internal/reedengine/lock.go
+++ b/internal/reedengine/lock.go
@@ -10,6 +10,7 @@ import (
 	"fmt"
 	"os"
 	"path/filepath"
+	"testing"
 
 	"github.com/Knatte18/loomyard/internal/lock"
 	"github.com/Knatte18/loomyard/internal/logger"
@@ -34,6 +35,13 @@ type Engine struct {
 	cfg  Config
 	geom Geometry
 	tmux TmuxCmd
+	// suppressHeaderLaunch decides whether ensureHeaderPaneLocked leaves the header pane as a bare
+	// shell instead of passing it a launch command. It is initialised from testing.Testing() because
+	// re-exec'ing os.Executable() from a test binary would run the whole suite recursively — but it
+	// lives as a field, not a hard-wired testing.Testing() call at the boot site, so an in-package
+	// test can flip it back on (see enableHeaderLaunch, lifecycle_test.go) and drive the real launch
+	// path against a fake tmux.
+	suppressHeaderLaunch bool
 }
 
 // New builds an Engine for the given Config and Geometry.
@@ -41,9 +49,10 @@ type Engine struct {
 // hubgeom.ReedGeometry is the hub-mode answer.
 func New(cfg Config, geom Geometry) *Engine {
 	return &Engine{
-		cfg:  cfg,
-		geom: geom,
-		tmux: NewTmuxCmd(cfg.Tmux, geom.SocketKey),
+		cfg:                  cfg,
+		geom:                 geom,
+		tmux:                 NewTmuxCmd(cfg.Tmux, geom.SocketKey),
+		suppressHeaderLaunch: testing.Testing(),
 	}
 }
 
diff --git a/internal/reedengine/render/height.go b/internal/reedengine/render/height.go
index afa0f29cd..3b3c1393a 100644
--- a/internal/reedengine/render/height.go
+++ b/internal/reedengine/render/height.go
@@ -96,7 +96,7 @@ func stackHeights(stack []Strand, box Box, p Params) []placement {
 
 	placements := make([]placement, n)
 	for i, s := range stack {
-		placements[i] = placement{id: s.PaneID, height: heights[i]}
+		placements[i] = placement{id: s.PaneID, height: heights[i], strip: isStrip[i]}
 	}
 	return placements
 }
diff --git a/internal/reedengine/render/layout.go b/internal/reedengine/render/layout.go
index 9a7087742..7ccb2a2a2 100644
--- a/internal/reedengine/render/layout.go
+++ b/internal/reedengine/render/layout.go
@@ -20,6 +20,12 @@ import (
 type placement struct {
 	id     string
 	height int
+	// strip reports whether this cell's height came from the collapsed-strip
+	// budget (an absolute row budget, p.CollapsedStripRows post-clamp) rather
+	// than from the equal-split of whatever rows were left. buildStackBody
+	// must not read it — it exists for FixedHeightPins (rules.go) to identify
+	// which placements to report.
+	strip bool
 }
 
 // buildStackBody renders panes into a tmux window_layout body positioned
diff --git a/internal/reedengine/render/pins_test.go b/internal/reedengine/render/pins_test.go
new file mode 100644
index 000000000..239f00ae2
--- /dev/null
+++ b/internal/reedengine/render/pins_test.go
@@ -0,0 +1,192 @@
+// pins_test.go pins FixedHeightPins against the heights Rules places for the same inputs, so the two
+// can never drift apart. Every case calls both entry points on the identical (strands, box, params)
+// triple: the expected pin list is asserted directly, and each returned pin's height is additionally
+// re-parsed out of Rules' own layout string rather than restated from the expectation — the second
+// check is what makes this a drift guard rather than a second copy of the policy.
+
+package render
+
+import (
+	"regexp"
+	"strconv"
+	"strings"
+	"testing"
+
+	"github.com/google/go-cmp/cmp"
+)
+
+// pinCellPattern matches one cell of a tmux window_layout body — "<w>x<h>,<x>,<y>,<id>" — capturing
+// the height and the bare pane id (a GROUP header has no trailing id field and so never matches).
+var pinCellPattern = regexp.MustCompile(`\d+x(\d+),\d+,\d+,([^,\]]+)`)
+
+// paneHeightFromLayout returns the height of paneID's cell within layout, parsed directly out of the
+// rendered window_layout string rather than recomputed from policy.
+func paneHeightFromLayout(t *testing.T, layout, paneID string) int {
+	t.Helper()
+	want := strings.TrimPrefix(paneID, "%")
+	for _, m := range pinCellPattern.FindAllStringSubmatch(layout, -1) {
+		if m[2] != want {
+			continue
+		}
+		height, err := strconv.Atoi(m[1])
+		if err != nil {
+			t.Fatalf("pane %q height %q did not parse as an integer: %v", paneID, m[1], err)
+		}
+		return height
+	}
+	t.Fatalf("pane %q not found as a cell in layout %q", paneID, layout)
+	return 0
+}
+
+// twoFullSiblings returns two co-equal, non-shrinking below-parent strands with no strip anywhere in
+// the stack — the fixture the no-strip cases build on.
+func twoFullSiblings() []Strand {
+	return []Strand{
+		{GUID: "a", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent}},
+		{GUID: "b", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
+	}
+}
+
+// twoDistinctStrips returns a root->mid->active chain where BOTH root and mid collapse to a strip —
+// each is an ancestor of the present active descendant — leaving active as the sole full pane. This
+// is the fixture the ordering test uses to prove two strip pins both follow the header pin.
+func twoDistinctStrips() []Strand {
+	return []Strand{
+		{GUID: "root", Parent: "", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent, ShrinkWhenWaitingOnChild: true}},
+		{GUID: "mid", Parent: "root", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent, ShrinkWhenWaitingOnChild: true}},
+		{GUID: "active", Parent: "mid", PaneID: "%3", Live: true, Display: Display{Anchor: AnchorBelowParent}},
+	}
+}
+
+func TestFixedHeightPinsMatchesRulesPlacedHeights(t *testing.T) {
+	tests := []struct {
+		name     string
+		strands  []Strand
+		box      Box
+		params   Params
+		wantPins []Pin
+		wantErr  bool
+	}{
+		{
+			name:     "HeaderPlusTwoFullStrandsOnlyTheHeaderIsPinned",
+			strands:  twoFullSiblings(),
+			box:      Box{X: 0, Y: 0, W: 100, H: 21},
+			params:   Params{CollapsedStripRows: 2, MinFullRows: 3, Header: Header{PaneID: "%h", HeightRows: 2}},
+			wantPins: []Pin{{PaneID: "%h", Height: 2}},
+		},
+		{
+			// Mirrors rules_test.go's TestRulesHeaderBandEnumeratesHeaderPlusEveryStrandCell fixture:
+			// header unclamped at 3, mid collapses to CollapsedStripRows (2).
+			name:     "HeaderPlusShrinkAncestorWithPresentDescendantHeaderThenStrip",
+			strands:  belowParentChain(),
+			box:      Box{X: 0, Y: 0, W: 100, H: 21},
+			params:   Params{CollapsedStripRows: 2, MinFullRows: 3, Header: Header{PaneID: "%h", HeightRows: 3}},
+			wantPins: []Pin{{PaneID: "%h", Height: 3}, {PaneID: "%2", Height: 2}},
+		},
+		{
+			// Mirrors TestRulesGolden's BelowParentFormsBottomDominantStackOrderedByParentChain
+			// fixture with no header configured: only the strip (mid) is pinned.
+			name:     "NoHeaderConfiguredWithStripPresentOnlyTheStripIsPinned",
+			strands:  belowParentChain(),
+			box:      Box{X: 0, Y: 0, W: 100, H: 21},
+			params:   Params{CollapsedStripRows: 2, MinFullRows: 3},
+			wantPins: []Pin{{PaneID: "%2", Height: 2}},
+		},
+		{
+			name:     "NoHeaderAndNoStripYieldsNoPins",
+			strands:  twoFullSiblings(),
+			box:      Box{X: 0, Y: 0, W: 100, H: 21},
+			params:   Params{CollapsedStripRows: 2, MinFullRows: 3},
+			wantPins: nil,
+		},
+		{
+			// headerRows=25 requested; clampHeaderHeight(25, box.H-1=20, MinFullRows=3) clamps to
+			// windowRows-floor=17 to preserve the stack's floor — the pin must carry 17, never the
+			// configured 25.
+			name:     "OversizedHeaderHeightRowsPinCarriesTheClampedValueNotConfigured",
+			strands:  twoFullSiblings(),
+			box:      Box{X: 0, Y: 0, W: 100, H: 21},
+			params:   Params{CollapsedStripRows: 2, MinFullRows: 3, Header: Header{PaneID: "%h", HeightRows: 25}},
+			wantPins: []Pin{{PaneID: "%h", Height: 17}},
+		},
+		{
+			// Mirrors rules_test.go's HeaderPresentClampedRowNoCellEverNonPositive golden row: the
+			// window is too short for the strip's natural CollapsedStripRows (2), and clampToFit's
+			// priority-1 pass reclaims it down to 1 — the pin must carry 1, never CollapsedStripRows.
+			name:     "TooShortWindowStripPinCarriesTheReclaimedValueNotCollapsedStripRows",
+			strands:  belowParentChain(),
+			box:      Box{X: 0, Y: 0, W: 100, H: 8},
+			params:   Params{CollapsedStripRows: 2, MinFullRows: 3, Header: Header{PaneID: "%h", HeightRows: 2}},
+			wantPins: []Pin{{PaneID: "%h", Height: 2}, {PaneID: "%2", Height: 1}},
+		},
+		{
+			// The sole-header branch: a header configured with no strand placed claims the whole box
+			// and has no absolute budget of its own — a stale one-row pin must never be emitted.
+			name:     "HeaderConfiguredWithNoStrandPlacedYieldsNoPin",
+			strands:  nil,
+			box:      Box{X: 0, Y: 0, W: 100, H: 21},
+			params:   Params{CollapsedStripRows: 2, MinFullRows: 3, Header: Header{PaneID: "%h", HeightRows: 3}},
+			wantPins: nil,
+		},
+		{
+			name: "AnchorOwnWindowYieldsNilNotAPanicMatchingRulesError",
+			strands: []Strand{
+				{GUID: "a", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorOwnWindow}},
+			},
+			box:      Box{X: 0, Y: 0, W: 100, H: 21},
+			params:   Params{CollapsedStripRows: 2, MinFullRows: 3},
+			wantPins: nil,
+			wantErr:  true,
+		},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			gotPins := FixedHeightPins(tt.strands, tt.box, tt.params)
+			if diff := cmp.Diff(tt.wantPins, gotPins); diff != "" {
+				t.Errorf("FixedHeightPins() mismatch (-want +got):\n%s", diff)
+			}
+
+			layout, _, err := Rules(tt.strands, tt.box, tt.params, nil)
+			if tt.wantErr {
+				if err == nil {
+					t.Fatalf("Rules() with the same input: expected error, got nil")
+				}
+				return
+			}
+			if err != nil {
+				t.Fatalf("Rules() unexpected error: %v", err)
+			}
+
+			// Assertion (b): every pin's height must equal the height that
+			// pane's cell actually carries in Rules' own layout string,
+			// parsed out of the string rather than restated from the
+			// expectation above.
+			for _, pin := range gotPins {
+				if got := paneHeightFromLayout(t, layout, pin.PaneID); got != pin.Height {
+					t.Errorf("pane %q: FixedHeightPins reported height %d, but Rules placed it at %d", pin.PaneID, pin.Height, got)
+				}
+			}
+		})
+	}
+}
+
+// TestFixedHeightPinsOrdersTheHeaderPinFirstThenEveryStripPin asserts pin ordering directly: with a
+// header and two distinct strips present, the header pin is index 0 and both strip pins follow.
+func TestFixedHeightPinsOrdersTheHeaderPinFirstThenEveryStripPin(t *testing.T) {
+	params := Params{CollapsedStripRows: 2, MinFullRows: 3, Header: Header{PaneID: "%h", HeightRows: 2}}
+	box := Box{X: 0, Y: 0, W: 100, H: 21}
+
+	pins := FixedHeightPins(twoDistinctStrips(), box, params)
+	if len(pins) != 3 {
+		t.Fatalf("FixedHeightPins() returned %d pins, want 3 (header + two strips): %+v", len(pins), pins)
+	}
+	if pins[0].PaneID != "%h" {
+		t.Errorf("pins[0].PaneID = %q, want the header pane %q first", pins[0].PaneID, "%h")
+	}
+	gotStrips := map[string]bool{pins[1].PaneID: true, pins[2].PaneID: true}
+	wantStrips := map[string]bool{"%1": true, "%2": true}
+	if diff := cmp.Diff(wantStrips, gotStrips); diff != "" {
+		t.Errorf("strip pins after the header (-want +got):\n%s", diff)
+	}
+}
diff --git a/internal/reedengine/render/rules.go b/internal/reedengine/render/rules.go
index 75106f615..7715b3abe 100644
--- a/internal/reedengine/render/rules.go
+++ b/internal/reedengine/render/rules.go
@@ -9,18 +9,39 @@ import (
 	"strings"
 )
 
-// Rules computes the tmux window_layout string and focus pane id for strands laid out within box.
-// It rejects any strand declaring AnchorOwnWindow, repairs corrupt cyclic parent chains, and drops
-// any strand whose PaneID is already spoken for by the header band or by an earlier strand
-// (see removeDuplicatePaneCells for why emitting one pane number twice is destructive).
-// When p.Header.PaneID is non-empty, Rules carves a fixed-height top band for the header before
-// laying out the stack below.
-// paneOrder resequences cells to match physical pane position;
-// a nil paneOrder keeps the intended (parent above child) order.
-func Rules(strands []Strand, box Box, p Params, paneOrder []string) (layout string, focus string, err error) {
+// cellPlan is the policy-layer result Rules and FixedHeightPins share: everything decided about
+// where strands land and how tall they are, before the mechanics layer (layout.go) turns it into a
+// tmux window_layout string. planCells builds it; Rules and FixedHeightPins each read the parts they
+// need and perform no policy of their own.
+type cellPlan struct {
+	// hasHeader reports whether p.Header.PaneID is non-empty — a header band
+	// is being rendered at all.
+	hasHeader bool
+	// soleHeader reports whether the header claims the whole box as a
+	// bracket-less single-cell body because no strand was placed. When true,
+	// headerHeight, stackBox, ordered, and placements carry no meaning.
+	soleHeader bool
+	// headerHeight is the header band's height after clampHeaderHeight,
+	// valid only when hasHeader is true and soleHeader is false.
+	headerHeight int
+	// stackBox is the region the strand stack is laid out within, below the
+	// header band and its one-row divider when hasHeader is true.
+	stackBox Box
+	// ordered is the below-parent stack, filtered and ordered by parent-chain
+	// depth.
+	ordered []Strand
+	// placements is ordered's per-strand height assignment from stackHeights.
+	placements []placement
+}
+
+// planCells performs the policy half of Rules: filtering and ordering strands into the below-parent
+// stack, and deciding the header/stack height split. It returns the same AnchorOwnWindow rejection
+// error Rules has always returned. Rules and FixedHeightPins are the mechanics layer built on top of
+// this shared policy result.
+func planCells(strands []Strand, box Box, p Params) (cellPlan, error) {
 	for _, s := range strands {
 		if s.Display.Anchor == AnchorOwnWindow {
-			return "", "", fmt.Errorf("render: strand %s uses deferred anchor %q", s.GUID, AnchorOwnWindow)
+			return cellPlan{}, fmt.Errorf("render: strand %s uses deferred anchor %q", s.GUID, AnchorOwnWindow)
 		}
 	}
 
@@ -33,11 +54,10 @@ func Rules(strands []Strand, box Box, p Params, paneOrder []string) (layout stri
 	hasHeader := p.Header.PaneID != ""
 	if hasHeader && len(ordered) == 0 {
 		// No strand placed: the header claims the whole box as a
-		// bracket-less single-cell body (see the doc comment) — never a
+		// bracket-less single-cell body (see Rules' doc comment) — never a
 		// zero-height cell inside a group, which the real multiplexer
 		// mishandles. No focus target exists without a placed strand.
-		sole := fmt.Sprintf("%dx%d,%d,%d,%s", box.W, box.H, box.X, box.Y, strings.TrimPrefix(p.Header.PaneID, "%"))
-		return wrapLayout(sole), "", nil
+		return cellPlan{hasHeader: true, soleHeader: true}, nil
 	}
 
 	stackBox := box
@@ -59,16 +79,86 @@ func Rules(strands []Strand, box Box, p Params, paneOrder []string) (layout stri
 	}
 
 	placements := stackHeights(ordered, stackBox, p)
-	placements = resequenceByPaneOrder(placements, paneOrder)
 
-	body := buildStackBody(stackBox, placements)
-	if hasHeader {
-		body = bandHeader(box, p.Header.PaneID, headerHeight, body)
+	return cellPlan{
+		hasHeader:    hasHeader,
+		headerHeight: headerHeight,
+		stackBox:     stackBox,
+		ordered:      ordered,
+		placements:   placements,
+	}, nil
+}
+
+// Rules computes the tmux window_layout string and focus pane id for strands laid out within box.
+// It rejects any strand declaring AnchorOwnWindow, repairs corrupt cyclic parent chains, and drops
+// any strand whose PaneID is already spoken for by the header band or by an earlier strand
+// (see removeDuplicatePaneCells for why emitting one pane number twice is destructive).
+// When p.Header.PaneID is non-empty, Rules carves a fixed-height top band for the header before
+// laying out the stack below.
+// paneOrder resequences cells to match physical pane position;
+// a nil paneOrder keeps the intended (parent above child) order.
+func Rules(strands []Strand, box Box, p Params, paneOrder []string) (layout string, focus string, err error) {
+	plan, err := planCells(strands, box, p)
+	if err != nil {
+		return "", "", err
+	}
+
+	if plan.soleHeader {
+		sole := fmt.Sprintf("%dx%d,%d,%d,%s", box.W, box.H, box.X, box.Y, strings.TrimPrefix(p.Header.PaneID, "%"))
+		return wrapLayout(sole), "", nil
+	}
+
+	placements := resequenceByPaneOrder(plan.placements, paneOrder)
+
+	body := buildStackBody(plan.stackBox, placements)
+	if plan.hasHeader {
+		body = bandHeader(box, p.Header.PaneID, plan.headerHeight, body)
 	}
-	focus = focusTarget(ordered)
+	focus = focusTarget(plan.ordered)
 	return wrapLayout(body), focus, nil
 }
 
+// Pin is one pane whose height is an absolute row budget rather than "whatever is left" — the header
+// band or a collapsed strip. Height is the height Rules actually placed the cell at, after
+// clampHeaderHeight/clampToFit — never the raw configured budget (p.Header.HeightRows or
+// p.CollapsedStripRows read directly), since either can yield rows under a too-short window.
+type Pin struct {
+	// PaneID is the tmux pane id this pin applies to.
+	PaneID string
+	// Height is the row height Rules placed this pane's cell at.
+	Height int
+}
+
+// FixedHeightPins reports the panes whose heights are absolute row budgets — the header band and
+// every collapsed strip — at the heights Rules actually placed them at for the identical
+// (strands, box, p) inputs. It shares Rules' own policy composition (planCells) so the two can never
+// disagree about a placed height.
+//
+// FixedHeightPins takes no paneOrder: a pin names its pane by tmux pane id, so emission order carries
+// no geometry — paneOrder only resequences layout-string cells, which FixedHeightPins never produces.
+//
+// It is pure and total like Rules. On any error from planCells, on the sole-header shape (there the
+// header claims the whole box and has no absolute budget of its own), or whenever there is otherwise
+// nothing to report, it returns nil. A caller must treat a nil return as "nothing is pinned", never
+// as "no opinion" — the disposition is exactly as authoritative as a non-nil one.
+func FixedHeightPins(strands []Strand, box Box, p Params) []Pin {
+	plan, err := planCells(strands, box, p)
+	if err != nil || plan.soleHeader {
+		return nil
+	}
+
+	var pins []Pin
+	if plan.hasHeader {
+		pins = append(pins, Pin{PaneID: p.Header.PaneID, Height: plan.headerHeight})
+	}
+	for _, pl := range plan.placements {
+		if pl.strip {
+			pins = append(pins, Pin{PaneID: pl.id, Height: pl.height})
+		}
+	}
+	return pins
+}
+
 // resequenceByPaneOrder reorders placements to follow paneOrder.
 // Each placement keeps its pane id and height; only the emission order
 // changes so buildStackBody recomputes y offsets correctly.
diff --git a/internal/reedengine/windowsize.go b/internal/reedengine/windowsize.go
index aaa18c3a3..9b98c886a 100644
--- a/internal/reedengine/windowsize.go
+++ b/internal/reedengine/windowsize.go
@@ -9,6 +9,7 @@ package reedengine
 
 import (
 	"errors"
+	"fmt"
 	"io/fs"
 	"os"
 	"runtime"
@@ -183,3 +184,62 @@ func (e *Engine) readWindowSizeLatestLocked() bool {
 	}
 	return windowSizeAllowsChain(out)
 }
+
+// resizePinHookArgvs returns the full argv sequence rebuilding session's `window-resized` window-hook
+// array for pins. It performs no I/O and no logging.
+//
+// The first returned argv is always the clear — {"set-hook", "-u", "-w", "-t",
+// exactSessionWindowTarget(session), "window-resized"} — emitted even when pins is empty, per the
+// Shared Decision the-clear-is-unconditional-including-zero-pins. Then one argv per pin, in pins
+// order: {"set-hook", "-w", "-t", exactSessionWindowTarget(session), "window-resized", body} for the
+// first pin and {"set-hook", "-a", "-w", "-t", exactSessionWindowTarget(session), "window-resized",
+// body} for every subsequent pin, where body is the single string "resize-pane -t <pane> -y
+// <height>".
+//
+// The body is one whole argv element; this function never emits a bare ";" element, because
+// set-hook takes its body as a single argument and a separate ";" element would terminate the
+// set-hook command itself. The array encoding — rather than one ";"-separated command string — exists
+// for failure isolation: verified live on tmux 3.6, a resize-pane naming a destroyed pane aborts the
+// rest of a single command list, while array entries are independent. The header is always pin index
+// 0 so it fires before any strip pin can go wrong.
+func resizePinHookArgvs(session string, pins []render.Pin) [][]string {
+	target := exactSessionWindowTarget(session)
+	argvs := make([][]string, 0, len(pins)+1)
+	argvs = append(argvs, []string{"set-hook", "-u", "-w", "-t", target, "window-resized"})
+	for i, pin := range pins {
+		body := fmt.Sprintf("resize-pane -t %s -y %d", pin.PaneID, pin.Height)
+		if i == 0 {
+			argvs = append(argvs, []string{"set-hook", "-w", "-t", target, "window-resized", body})
+		} else {
+			argvs = append(argvs, []string{"set-hook", "-a", "-w", "-t", target, "window-resized", body})
+		}
+	}
+	return argvs
+}
+
+// installResizePinsLocked rebuilds this session's `window-resized` window-hook array from pins,
+// issuing each argv resizePinHookArgvs builds through e.tmux.run. It returns nothing.
+//
+// This follows the Shared Decision hook-failure-is-non-fatal-everywhere, which already governs
+// pinGeometryOptionsLocked in this same file: each failure is logged via logger.Warn naming the
+// socket, the session and the error, and then ignored, so a failed call never stops the calls after
+// it — a failed clear still lets the rebuild proceed, since the first (non-"-a") set-hook overwrites
+// the array from entry [0] regardless.
+//
+// The clear is unconditional because reaching a call site means reed has computed an opinion, and
+// with zero pins that opinion is "nothing is pinned" (Shared Decision
+// the-clear-is-unconditional-including-zero-pins). The whole array is a snapshot rebuilt on every
+// successful apply rather than something recomputed at fire time.
+//
+// Known limitation: a clamp-derived pin is computed for the box at install time, so an operator who
+// shrinks the terminal past a clamp threshold with no intervening reed op keeps a pre-shrink pin,
+// bounded by tmux's own one-row floor and self-correcting on the next reed op.
+//
+// Assumes the op lock is already held, like every other Locked method in this file.
+func (e *Engine) installResizePinsLocked(pins []render.Pin) {
+	for _, argv := range resizePinHookArgvs(e.SessionName(), pins) {
+		if err := e.tmux.run(argv...); err != nil {
+			logger.Warn("reed: failed to install resize-pane hook", "socket", e.Socket(), "session", e.SessionName(), "err", err)
+		}
+	}
+}
diff --git a/internal/reedengine/windowsize_test.go b/internal/reedengine/windowsize_test.go
index 5f0b1a5cb..4fc010600 100644
--- a/internal/reedengine/windowsize_test.go
+++ b/internal/reedengine/windowsize_test.go
@@ -6,6 +6,7 @@ package reedengine
 
 import (
 	"errors"
+	"fmt"
 	"io/fs"
 	"os"
 	"path/filepath"
@@ -376,3 +377,106 @@ func containsArg(args []string, want string) bool {
 	}
 	return false
 }
+
+// TestResizePinHookArgvs pins the pure argv shape resizePinHookArgvs builds for zero, one, and
+// several pins: the unconditional clear always leads, every argv carries -w and the exact-match
+// window target, each body is exactly "resize-pane -t <pane> -y <height>", the "-a" flag appears on
+// every entry after the first pin, and no argv anywhere in the sequence carries a bare ";" element.
+func TestResizePinHookArgvs(t *testing.T) {
+	const session = "myproj"
+	target := exactSessionWindowTarget(session)
+
+	assertCommon := func(t *testing.T, argvs [][]string) {
+		t.Helper()
+		if len(argvs) == 0 {
+			t.Fatal("resizePinHookArgvs() = empty slice, want at least the clear")
+		}
+		for i, argv := range argvs {
+			if argv[0] != "set-hook" {
+				t.Errorf("argv[%d][0] = %q, want %q", i, argv[0], "set-hook")
+			}
+			if !containsArg(argv, "-w") {
+				t.Errorf("argv[%d] = %v, want -w", i, argv)
+			}
+			if !containsArg(argv, target) {
+				t.Errorf("argv[%d] = %v, want the exact-match window target %q", i, argv, target)
+			}
+			if !containsArg(argv, "window-resized") {
+				t.Errorf("argv[%d] = %v, want the window-resized hook name", i, argv)
+			}
+			for _, elem := range argv {
+				if elem == ";" {
+					t.Errorf("argv[%d] = %v, want no bare \";\" element", i, argv)
+				}
+			}
+		}
+	}
+
+	t.Run("ZeroPins", func(t *testing.T) {
+		argvs := resizePinHookArgvs(session, nil)
+		assertCommon(t, argvs)
+		if len(argvs) != 1 {
+			t.Fatalf("resizePinHookArgvs(zero pins) = %v, want exactly one argv (the clear)", argvs)
+		}
+		want := []string{"set-hook", "-u", "-w", "-t", target, "window-resized"}
+		if len(argvs[0]) != len(want) {
+			t.Fatalf("argv[0] = %v, want %v", argvs[0], want)
+		}
+		for i := range want {
+			if argvs[0][i] != want[i] {
+				t.Errorf("argv[0][%d] = %q, want %q", i, argvs[0][i], want[i])
+			}
+		}
+	})
+
+	t.Run("OnePin", func(t *testing.T) {
+		pins := []render.Pin{{PaneID: "%1", Height: 3}}
+		argvs := resizePinHookArgvs(session, pins)
+		assertCommon(t, argvs)
+		if len(argvs) != 2 {
+			t.Fatalf("resizePinHookArgvs(1 pin) = %v, want 2 argvs (clear + 1)", argvs)
+		}
+		if containsArg(argvs[0], "-a") {
+			t.Errorf("clear argv = %v, want no -a", argvs[0])
+		}
+		if containsArg(argvs[1], "-a") {
+			t.Errorf("first-pin argv = %v, want no -a on the non-a set-hook", argvs[1])
+		}
+		wantBody := "resize-pane -t %1 -y 3"
+		if argvs[1][len(argvs[1])-1] != wantBody {
+			t.Errorf("first-pin body = %q, want %q", argvs[1][len(argvs[1])-1], wantBody)
+		}
+	})
+
+	t.Run("ThreePins", func(t *testing.T) {
+		pins := []render.Pin{
+			{PaneID: "%1", Height: 3},
+			{PaneID: "%2", Height: 2},
+			{PaneID: "%3", Height: 4},
+		}
+		argvs := resizePinHookArgvs(session, pins)
+		assertCommon(t, argvs)
+		if len(argvs) != 4 {
+			t.Fatalf("resizePinHookArgvs(3 pins) = %v, want 4 argvs (clear + 3)", argvs)
+		}
+		if containsArg(argvs[0], "-a") {
+			t.Errorf("clear argv = %v, want no -a", argvs[0])
+		}
+		if containsArg(argvs[1], "-a") {
+			t.Errorf("first-pin argv = %v, want no -a", argvs[1])
+		}
+		for i, want := range []struct {
+			pane   string
+			height int
+		}{{"%1", 3}, {"%2", 2}, {"%3", 4}} {
+			argv := argvs[i+1]
+			if i > 0 && !containsArg(argv, "-a") {
+				t.Errorf("argv for pin %d = %v, want -a", i, argv)
+			}
+			wantBody := fmt.Sprintf("resize-pane -t %s -y %d", want.pane, want.height)
+			if argv[len(argv)-1] != wantBody {
+				t.Errorf("argv for pin %d body = %q, want %q", i, argv[len(argv)-1], wantBody)
+			}
+		}
+	})
+}
diff --git a/mill-config.yaml b/mill-config.yaml
index e1673881a..7d77af161 100644
--- a/mill-config.yaml
+++ b/mill-config.yaml
@@ -140,7 +140,7 @@ pipeline:
 roles:
   discussion-review:
     holistic:
-      rounds: 5
+      rounds: 6
       reviewer: opusmedium
       # large_prompt:            # optional: override reviewer for large prompts
       #   threshold_ktok: 100    # switch when estimated tok count >= this (char/4000)
@@ -151,7 +151,7 @@ roles:
       rounds: 0
       reviewer: null
     holistic:
-      rounds: 7
+      rounds: 6
       reviewer: sonnetxhigh
       # large_prompt:            # optional: override reviewer for large prompts
       #   threshold_ktok: 100    # switch when estimated tok count >= this (char/4000)

```

## Instructions

1. Read the failing tests and the source files they exercise.
2. Fix the root cause of the failures.
   Do not modify tests unless they are genuinely wrong due to the merge (e.g. a test asserted against a value that the merge legitimately changed).
3. Re-run `go test ./internal/shell/... ./internal/reedengine/...` after each fix attempt using `git -C /home/knatte/Code/loomyard/wts/reed-watchdog-daemon` for git commands.
4. Commit each fix attempt with a clear commit message.
5. Self-fix up to `3` times.
   If the verify command still fails after `3` attempts, stop and report stuck.

## Report

Your last output line MUST be a bare JSON object (no code fence, no backticks):

**`commit_sha` MUST be the full SHA from `git rev-parse HEAD` -- never the abbreviated form (`git rev-parse --short HEAD`) or a `git log --oneline` hash.**

On success:

{"status":"success","commit_sha":"<last-HEAD-sha>"}

After exhausting fix rounds:

{"status":"stuck","stuck_type":"verify","reason":"<one-line description of what still fails>","commit_sha":"<last-HEAD-sha>"}

Anything other than this JSON object on the last line is a protocol violation;
the merge-in dispatcher treats that as stuck_type: logic with reason "no structured report" — your work is lost.
Do not wrap the JSON in a code fence;
do not add commentary after it.

## Tools

Available: Read, Edit, Write, Bash, Grep, Glob.
Use `git -C /home/knatte/Code/loomyard/wts/reed-watchdog-daemon` for git commands;
do not `cd`.
Worktree cwd is `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon`.

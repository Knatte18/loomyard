// report.go defines the sandbox-report.json contract shared with the launcher's caller (the
// loomyard tooling described in millhouse#586) and implements fetchReport, which reads the
// agent-written report out of the Hub warp repo, validates and stamps it, then writes a normalized,
// fingerprint-stamped copy into the loomyard root's .scratch directory.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Sandbox report file naming.
const (
	reportFileName = "sandbox-report.json"
	reportSourceID = "sandbox-report"
)

// sandboxReport is the top-level shape of sandbox-report.json.
type sandboxReport struct {
	Source string        `json:"source"`
	Meta   reportMeta    `json:"meta"`
	Items  *[]reportItem `json:"items"`
}

// reportMeta holds provenance metadata attached to a sandboxReport.
type reportMeta struct {
	Fingerprint reportFingerprint `json:"fingerprint"`
}

// reportFingerprint identifies the exact lyx binary that produced a report.
type reportFingerprint struct {
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
	ModTime string `json:"modtime"`
	Source  string `json:"source"`
}

// reportItem is a single WARN/FAIL finding recorded by the agent during a
// sandbox session.
type reportItem struct {
	Ref   string `json:"ref"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

// runFetch executes "sandbox fetch" after a suite session. Re-resolves lyx,
// re-fingerprints it, and fetches agent-written sandbox-report.json.
func runFetch(parentDir, loomyardRoot string) error {
	warpRepoDir := filepath.Join(parentDir, hubName, warpDirName)

	if _, err := os.Stat(warpRepoDir); os.IsNotExist(err) {
		return fmt.Errorf("hub warp repo not found at %s -- run sandbox/build.cmd first", warpRepoDir)
	} else if err != nil {
		return fmt.Errorf("stat warp repo %s: %w", warpRepoDir, err)
	}

	lyxPath, source, err := resolveLyx()
	if err != nil {
		return err
	}

	info, err := binaryFingerprint(lyxPath, source)
	if err != nil {
		return fmt.Errorf("fingerprint lyx binary: %w", err)
	}

	destPath, count, err := fetchReport(warpRepoDir, loomyardRoot, info)
	if err != nil {
		return err
	}
	if count == 0 {
		// Nothing to triage; no Next step to point at.
		fmt.Printf("fetched 0 finding(s) -> %q (clean run -- nothing to triage)\n", destPath)
		return nil
	}
	// Point the operator at the concrete triage skill, quoting the path so it
	// survives spaces when pasted into the /mill-report-to-tasks invocation.
	fmt.Printf("fetched %d finding(s) -> %q\n\n"+
		"Next: /mill-report-to-tasks %q\n"+
		"      (groups the findings into wiki tasks; nothing is written until you approve)\n",
		count, destPath, destPath)
	return nil
}

// fetchReport reads sandbox-report.json, validates it, stamps meta.fingerprint,
// and writes the normalized result to .scratch. Returns the written path and
// finding count.
func fetchReport(warpRepoDir, loomyardRoot string, info binaryInfo) (string, int, error) {
	reportPath := filepath.Join(warpRepoDir, reportFileName)

	raw, err := os.ReadFile(reportPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", 0, fmt.Errorf("sandbox report not found at %s: the agent produced no report", reportPath)
		}
		return "", 0, fmt.Errorf("read sandbox report %s: %w", reportPath, err)
	}

	var report sandboxReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return "", 0, fmt.Errorf("parse sandbox report %s: %w", reportPath, err)
	}

	if report.Source != reportSourceID {
		return "", 0, fmt.Errorf("sandbox report has wrong source %q (want %q)", report.Source, reportSourceID)
	}
	if report.Items == nil {
		return "", 0, fmt.Errorf("sandbox report is missing its items array")
	}

	report.Meta.Fingerprint = reportFingerprint{
		Path:    info.Path,
		SHA256:  info.SHA256,
		Size:    info.Size,
		ModTime: info.ModTime.Format(time.RFC3339),
		Source:  info.Source,
	}

	normalized, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", 0, fmt.Errorf("marshal normalized sandbox report: %w", err)
	}

	scratchDir := filepath.Join(loomyardRoot, ".scratch")
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return "", 0, fmt.Errorf("create scratch dir %s: %w", scratchDir, err)
	}

	destPath := filepath.Join(scratchDir, "sandbox-report-"+info.SHA256+".json")
	if err := os.WriteFile(destPath, normalized, 0o644); err != nil {
		return "", 0, fmt.Errorf("write fetched sandbox report %s: %w", destPath, err)
	}

	return destPath, len(*report.Items), nil
}

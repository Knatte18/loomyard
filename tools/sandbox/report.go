// report.go defines the sandbox-report.json contract shared with the launcher's
// caller (the loomyard tooling described in millhouse#586) and implements
// fetchReport, which reads the agent-written report out of the Hub host repo,
// validates and stamps it, then writes a normalized, fingerprint-stamped copy
// into the loomyard root's .scratch directory.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Sandbox report file naming. reportFileName is the agent-written file inside
// the Hub host repo; reportSourceID is the required value of the report's
// top-level "source" field, used to reject reports from an unrelated producer.
const (
	reportFileName = "sandbox-report.json"
	reportSourceID = "sandbox-report"
)

// sandboxReport is the top-level shape of sandbox-report.json. The agent
// writes only Source and Items; Meta is stamped by fetchReport from the
// authoritative binaryInfo during the fetch step (see the
// "suite.go stamps meta.fingerprint" Shared Decision).
type sandboxReport struct {
	Source string        `json:"source"`
	Meta   reportMeta    `json:"meta"`
	Items  *[]reportItem `json:"items"`
}

// reportMeta holds provenance metadata attached to a sandboxReport. Today it
// carries only the binary fingerprint, but is its own type so future
// provenance fields can be added without reshaping sandboxReport.
type reportMeta struct {
	Fingerprint reportFingerprint `json:"fingerprint"`
}

// reportFingerprint identifies the exact lyx binary that produced a report,
// mirroring the fields already captured by binaryInfo so a maintainer can
// trace a finding back to the build that triggered it.
type reportFingerprint struct {
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
	ModTime string `json:"modtime"`
}

// reportItem is a single WARN/FAIL finding recorded by the agent during a
// sandbox session.
type reportItem struct {
	Ref   string `json:"ref"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

// runFetch executes the "sandbox fetch" subcommand, run by the operator
// after a suite session ends. It mirrors runSuite's host-repo derivation,
// re-fingerprints the lyx currently on PATH, and fetches the agent-written
// sandbox-report.json into <loomyardRoot>/.scratch. For the normal flow (run the
// suite, then fetch) the on-PATH binary is the same one the suite fingerprinted,
// so re-fingerprinting here is acceptable and intended.
func runFetch(parentDir, loomyardRoot string) error {
	// Derive the host repo path the same way runSuite does, from the shared
	// hubName const (main.go) and the suite-local hostDirName const.
	hostRepoDir := filepath.Join(parentDir, hubName, hostDirName)

	// Guard against a missing Hub so the operator gets a clear, actionable message
	// rather than a confusing downstream read failure.
	if _, err := os.Stat(hostRepoDir); os.IsNotExist(err) {
		return fmt.Errorf("hub host repo not found at %s -- run sandbox-build.cmd first", hostRepoDir)
	} else if err != nil {
		return fmt.Errorf("stat host repo %s: %w", hostRepoDir, err)
	}

	// Resolve lyx via PATH so the fingerprint captures the exact binary the
	// operator has deployed; the binary must be on PATH before running the suite.
	lyxPath, err := lookPath("lyx")
	if err != nil {
		return fmt.Errorf("lyx not found on PATH -- deploy the binary before running the suite: %w", err)
	}

	info, err := binaryFingerprint(lyxPath)
	if err != nil {
		return fmt.Errorf("fingerprint lyx binary: %w", err)
	}

	destPath, count, err := fetchReport(hostRepoDir, loomyardRoot, info)
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

// fetchReport reads sandbox-report.json from hostRepoDir, validates it
// against the sandbox-report contract, stamps its meta.fingerprint from info,
// and writes the normalized result to
// <loomyardRoot>/.scratch/sandbox-report-<sha256>.json. It returns the written
// destination path and the number of findings (items) so the caller can report
// what it fetched. It is called by the separate fetch step (runFetch) after a
// suite session, per the "normalized re-serialize" Shared Decision.
func fetchReport(hostRepoDir, loomyardRoot string, info binaryInfo) (string, int, error) {
	reportPath := filepath.Join(hostRepoDir, reportFileName)

	raw, err := os.ReadFile(reportPath)
	if err != nil {
		if os.IsNotExist(err) {
			// A missing report means the agent finished without writing one --
			// surface this distinctly from a parse error so the operator knows
			// the agent itself misbehaved, not that the file was malformed.
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
	// Items is decoded as *[]reportItem so a nil pointer (key absent) can be
	// distinguished from a non-nil pointer to an empty slice (key present,
	// zero findings) -- see the "typed-decode validation with *[]Item" Shared
	// Decision. Only the former is rejected.
	if report.Items == nil {
		return "", 0, fmt.Errorf("sandbox report is missing its items array")
	}

	// The agent does not know its own fingerprint, so the fetch helper -- which
	// already has the authoritative binaryInfo -- stamps it here, overwriting
	// anything the agent may have written to meta.
	report.Meta.Fingerprint = reportFingerprint{
		Path:    info.Path,
		SHA256:  info.SHA256,
		Size:    info.Size,
		ModTime: info.ModTime.Format(time.RFC3339),
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

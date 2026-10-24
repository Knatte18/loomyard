// Package lensreport names the files a lens review's writer saves each lens fork's report in, so an audit reads every report from the run records.
// A report sits beside the record the writer records its lens review in, named from the record's stem: `<stem>.lens-<lens>.md`.
// PathPattern renders that name for a prompt, and IsReportName tells a record's reports from the other files in its directory.
// The writer session, not Go, writes each report, in the format the `twill-lens-report-spec` spec defines; SpecName names that spec.
// It imports the standard library only, so every lens review host and every check of a record's directory can share the naming.
package lensreport

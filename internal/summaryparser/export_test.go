package summaryparser

// Sentinels re-exported for the external test package's errors.Is assertions.
var (
	ErrEmptyFileForTest  = errEmptyFile
	ErrNoHeadingForTest  = errNoHeading
	ErrEmptyTitleForTest = errEmptyTitle
)

// Package formatters implements one formatter/linter per supported file
// type. Each formatter is either native (runs in-process, no subprocess) or
// external (shells out to a project-local or system tool via bunx/PATH).
package formatters

import "context"

// Result is the outcome of running a formatter against one file.
type Result struct {
	// Changed is true if the file's contents were modified.
	Changed bool
	// Skipped is true if the formatter declined to run (tool not found,
	// no config present, file outside its scope). Not an error.
	Skipped bool
	// Diagnostic is a short, human-readable failure message (already
	// truncated to a safe size). Empty on success or skip.
	Diagnostic string
	// Err is non-nil only for genuine unexpected failures. A tool
	// reporting lint/format problems on the file is NOT an error here —
	// callers must never propagate it as a hook failure.
	Err error
}

// Formatter formats or lints a single file in place.
type Formatter interface {
	// Name is used in diagnostic output, e.g. "biome", "gofmt-sh".
	Name() string
	// Format runs against the absolute path abs, rooted at projectRoot
	// (the value of $CLAUDE_PROJECT_DIR, or cwd as a fallback).
	Format(ctx context.Context, projectRoot, abs string) Result
}

package main

import (
	"testing"

	"github.com/go-quicktest/qt"
)

func TestCompletionAndManGeneration(t *testing.T) {
	var tops []string
	for _, c := range visibleCommands() {
		tops = append(tops, c.completionNames()...)
	}
	qt.Assert(t, qt.Not(qt.Equals(len(tops), 0)))

	cases := append(append([]string{}, completionShells...), "man")
	for _, kind := range cases {
		t.Run(kind, func(t *testing.T) {
			var code int
			var stdout string
			if kind == "man" {
				stdout = captureStdout(t, func() { code = dispatchArgs([]string{"man"}) })
			} else {
				stdout = captureStdout(t, func() { code = dispatchArgs([]string{"completion", kind}) })
			}
			qt.Check(t, qt.Equals(code, 0))
			qt.Check(t, qt.Not(qt.Equals(stdout, "")))
			for _, name := range tops {
				qt.Check(t, qt.StringContains(stdout, name), qt.Commentf("missing %q", name))
			}
		})
	}
}

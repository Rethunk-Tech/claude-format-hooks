package main

import (
	"path/filepath"
	"testing"

	"github.com/go-quicktest/qt"
)

func TestShellShebangExt(t *testing.T) {
	cases := []struct {
		name string
		src  string
		ext  string
		ok   bool
	}{
		{"LF env bash", "#!/usr/bin/env bash\n", ".sh", true},
		{"CRLF env bash", "#!/usr/bin/env bash\r\n", ".sh", true},
		{"direct sh", "#!/bin/sh\n", ".sh", true},
		{"direct bash", "#!/bin/bash\n", ".sh", true},
		{"non-shell python", "#!/usr/bin/env python\n", "", false},
		{"no shebang", "echo hello\n", "", false},
		{"empty", "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "script")
			writeFile(t, path, tc.src)

			ext, ok := shellShebangExt(path)
			qt.Check(t, qt.Equals(ext, tc.ext))
			qt.Check(t, qt.Equals(ok, tc.ok))
		})
	}
}

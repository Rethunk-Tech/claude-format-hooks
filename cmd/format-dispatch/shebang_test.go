package main

import (
	"path/filepath"
	"testing"

	"github.com/go-quicktest/qt"
)

func TestShebangExt(t *testing.T) {
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
		{"direct zsh", "#!/bin/zsh\n", ".sh", true},
		{"direct ksh", "#!/bin/ksh\n", ".sh", true},
		{"env python", "#!/usr/bin/env python\n", ".py", true},
		{"versioned python", "#!/usr/bin/python3.12\n", ".py", true},
		{"env -S python", "#!/usr/bin/env -S python3 -u\n", ".py", true},
		{"env -S bash", "#!/usr/bin/env -S bash -euo pipefail\n", ".sh", true},
		{"env --split-string python", "#!/usr/bin/env --split-string=python3\n", ".py", true},
		{"env -i python", "#!/usr/bin/env -i python3\n", ".py", true},
		{"env -u VAR python", "#!/usr/bin/env -u FOO python3\n", ".py", true},
		{"env NAME=VALUE python", "#!/usr/bin/env FOO=bar python3\n", ".py", true},
		{"env -S with no command", "#!/usr/bin/env -S\n", "", false},
		{"ruby", "#!/usr/bin/env ruby\n", ".rb", true},
		{"node", "#!/usr/bin/env node\n", ".js", true},
		{"bun", "#!/usr/bin/env bun\n", ".js", true},
		{"unknown interpreter", "#!/usr/bin/env perl\n", "", false},
		{"unknown after env -S", "#!/usr/bin/env -S perl -w\n", "", false},
		{"no shebang", "echo hello\n", "", false},
		{"empty", "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "script")
			writeFile(t, path, tc.src)

			ext, ok := shebangExt(path)
			qt.Check(t, qt.Equals(ext, tc.ext))
			qt.Check(t, qt.Equals(ok, tc.ok))
		})
	}
}

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
		{"hashbang only", "#!\n", "", false},
		{"env with no operand", "#!/usr/bin/env\n", "", false},
		{"env dash before python", "#!/usr/bin/env - python3\n", ".py", true},
		{"env -u equals form", "#!/usr/bin/env -u=FOO python3\n", ".py", true},
		{"env --chdir equals form", "#!/usr/bin/env --chdir=/tmp python3\n", ".py", true},
		{"env -u dangling", "#!/usr/bin/env -u\n", "", false},
		{"ruby", "#!/usr/bin/env ruby\n", ".rb", true},
		{"php", "#!/usr/bin/env php\n", ".php", true},
		{"lua", "#!/usr/bin/env lua\n", ".lua", true},
		{"luajit", "#!/usr/bin/env luajit\n", ".lua", true},
		{"node", "#!/usr/bin/env node\n", ".js", true},
		{"bun", "#!/usr/bin/env bun\n", ".js", true},
		{"deno", "#!/usr/bin/env deno\n", ".ts", true},
		{"env -S deno", "#!/usr/bin/env -S deno run\n", ".ts", true},
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

	t.Run("missing file", func(t *testing.T) {
		ext, ok := shebangExt(filepath.Join(t.TempDir(), "absent"))
		qt.Check(t, qt.Equals(ext, ""))
		qt.Check(t, qt.IsFalse(ok))
	})
}

func TestResolveDispatchExt(t *testing.T) {
	dir := t.TempDir()
	bashrc := filepath.Join(dir, ".bashrc")
	writeFile(t, bashrc, "#!/bin/bash\necho hi\n")
	qt.Check(t, qt.Equals(resolveDispatchExt(bashrc), ".sh"))

	unknown := filepath.Join(dir, "notes.xyz")
	writeFile(t, unknown, "#!/bin/bash\necho hi\n")
	qt.Check(t, qt.Equals(resolveDispatchExt(unknown), ".xyz"))

	zsh := filepath.Join(dir, "tool.zsh")
	writeFile(t, zsh, "echo hi\n")
	qt.Check(t, qt.Equals(resolveDispatchExt(zsh), ".zsh"))
}

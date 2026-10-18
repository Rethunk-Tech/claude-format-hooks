package formatters

import (
	"context"
	"go/format"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-quicktest/qt"
)

func TestGoFormatterName(t *testing.T) {
	qt.Check(t, qt.Equals(NewGo().Name(), "gofmt"))
}

func TestGoFormatterIdempotent(t *testing.T) {
	withoutGofumpt(t)
	cases := []struct {
		name string
		src  string
	}{
		{"already gofmt'd", "package main\n\nfunc main() {}\n"},
		{"messy braces and spacing", "package main\n\nfunc main(){\n    println(\"hi\")\n}\n"},
		{"spaces instead of tabs", "package main\n\ntype T struct {\n    A int\n    B string\n}\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, path := sourceFile(t, "t.go", tc.src)

			assertIdempotent(t, NewGo(), dir, path)
		})
	}
}

func TestGoFormatterReadErrorIsNotSkipped(t *testing.T) {
	assertDirectoryIsAReadError(t, NewGo())
}

func TestGoFormatterReformatsIndentation(t *testing.T) {
	withoutGofumpt(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "t.go")
	src := "package main\n\nfunc main(){\nprintln(\"hi\")\n}\n"
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(src), 0o600)))

	res := NewGo().Format(t.Context(), dir, path)
	qt.Check(t, qt.IsNil(res.Err))

	out := readSource(t, path)
	qt.Check(t, qt.StringContains(string(out), "\tprintln(\"hi\")"),
		qt.Commentf("expected tab-indented output, got %q", out))
}

func TestGoFormatterSkipsInvalidSyntax(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.go")
	src := "package main\nfunc {\n"
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(src), 0o600)))

	res := NewGo().Format(t.Context(), dir, path)
	qt.Check(t, qt.IsTrue(res.Skipped))

	out := readSource(t, path)
	qt.Check(t, qt.Equals(string(out), src), qt.Commentf("malformed source must not be modified"))
}

// withoutGofumpt hides every PATH binary and gives the miss cache a private
// directory, so these tests exercise go/format.Source even on a machine
// that has gofumpt installed.
func withoutGofumpt(t *testing.T) {
	t.Helper()
	isolateDiskCache(t)
	clearPath(t)
}

func writeFakeTools(t *testing.T, bodies map[string]string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake shell-script tools are POSIX-shell only")
	}
	dir := t.TempDir()
	for name, body := range bodies {
		path := filepath.Join(dir, name)
		script := "#!/bin/sh\n" + body + "\n"
		qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(script), 0o755))) //nolint:gosec // test formatter fixture must be executable
	}
	t.Setenv("PATH", dir)
}

// goFileUnderConfig writes src at pkg/t.go beneath a parent .golangci.yml.
// An empty yml writes no config file.
func goFileUnderConfig(t *testing.T, yml, src string) string {
	t.Helper()
	dir := t.TempDir()
	if yml != "" {
		qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, ".golangci.yml"), []byte(yml), 0o600)))
	}
	pkg := filepath.Join(dir, "pkg")
	qt.Assert(t, qt.IsNil(os.Mkdir(pkg, 0o750)))
	path := filepath.Join(pkg, "t.go")
	qt.Assert(t, qt.IsNil(os.WriteFile(path, []byte(src), 0o600)))
	return path
}

func argvHasLine(argv, want string) bool {
	for line := range strings.SplitSeq(argv, "\n") {
		if line == want {
			return true
		}
	}
	return false
}

const validGoSrc = "package main\n\nfunc main() {}\n"

// gofumptCopyScript records argv and copies the input through. A flag is
// not a path: group_params puts -extra=group_params ahead of the filename,
// and the stdin path (goimports already ran) has no filename at all.
const gofumptCopyScript = `printf '%s\n' "$@" > "$GO_TOOL_ARGV"
file=
for a in "$@"; do
  case $a in
    -*) ;;
    *) file=$a ;;
  esac
done
if [ -n "$file" ]; then
  /bin/cat "$file"
else
  /bin/cat
fi`

const gofumptOnlyYML = `version: "2"
formatters:
  enable:
    - gofumpt
`

const groupParamsYML = `version: "2"
formatters:
  enable:
    - gofumpt
  settings:
    gofumpt:
      extra:
        group-params: true
`

const goimportsAndGofumptYML = `version: "2"
formatters:
  enable:
    - gofumpt
    - goimports
  settings:
    gofumpt:
      extra:
        group-params: true
`

func TestGoFormatterFallsBackToGofmtWhenGofumptIsAbsent(t *testing.T) {
	withoutGofumpt(t)
	src := "package main\n\nfunc main(){\nprintln(\"hi\")\n}\n"
	dir, path := sourceFile(t, "t.go", src)

	res := NewGo().Format(t.Context(), dir, path)
	assertFormatted(t, res)

	want, err := format.Source([]byte(src))
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.DeepEquals(readSource(t, path), want))
}

func TestGoFormatterUsesGofumptWhenPresent(t *testing.T) {
	isolateDiskCache(t)
	argv := filepath.Join(t.TempDir(), "argv")
	t.Setenv("GO_TOOL_ARGV", argv)
	writeFakeTools(t, map[string]string{
		"gofumpt": `printf '%s\n' "$@" > "$GO_TOOL_ARGV"
printf 'package main\n\nfunc main() {}\n// gofumpt\n'`,
		"goimports": `printf 'goimports-ran\n' >> "$GO_TOOL_ARGV"
/bin/cat "$1"`,
	})
	path := goFileUnderConfig(t, gofumptOnlyYML, validGoSrc)

	res := NewGo().Format(t.Context(), filepath.Dir(path), path)
	assertFormatted(t, res)
	qt.Check(t, qt.Equals(string(readSource(t, path)), "package main\n\nfunc main() {}\n// gofumpt\n"))

	args := string(readSource(t, argv))
	qt.Check(t, qt.IsTrue(argvHasLine(args, path)), qt.Commentf("gofumpt should read the file, argv %q", args))
	qt.Check(t, qt.IsFalse(argvHasLine(args, "-w")), qt.Commentf("stdout, not -w: %q", args))
	qt.Check(t, qt.IsFalse(argvHasLine(args, "-extra=group_params")), qt.Commentf("no group_params without config: %q", args))
	qt.Check(t, qt.IsFalse(strings.Contains(args, "goimports-ran")), qt.Commentf("goimports is not enabled: %q", args))
}

func TestGoFormatterGroupParamsFollowsConfig(t *testing.T) {
	cases := []struct {
		name string
		yml  string
		want bool
	}{
		{"enabled", groupParamsYML, true},
		{"disabled", "version: \"2\"\nformatters:\n  enable:\n    - gofumpt\n  settings:\n    gofumpt:\n      extra:\n        group-params: false\n", false},
		{"omitted", gofumptOnlyYML, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateDiskCache(t)
			argv := filepath.Join(t.TempDir(), "argv")
			t.Setenv("GO_TOOL_ARGV", argv)
			writeFakeTools(t, map[string]string{
				"gofumpt": gofumptCopyScript,
			})
			path := goFileUnderConfig(t, tc.yml, validGoSrc)

			res := NewGo().Format(t.Context(), filepath.Dir(path), path)
			assertFormatted(t, res)

			args := string(readSource(t, argv))
			qt.Check(t, qt.Equals(argvHasLine(args, "-extra=group_params"), tc.want), qt.Commentf("argv %q", args))
			qt.Check(t, qt.IsFalse(argvHasLine(args, "-w")))
			qt.Check(t, qt.IsTrue(argvHasLine(args, path)))
		})
	}
}

func TestGoFormatterRunsGoimportsBeforeGofumpt(t *testing.T) {
	isolateDiskCache(t)
	logPath := filepath.Join(t.TempDir(), "log")
	argv := filepath.Join(t.TempDir(), "argv")
	t.Setenv("GO_TOOL_LOG", logPath)
	t.Setenv("GO_TOOL_ARGV", argv)
	writeFakeTools(t, map[string]string{
		"goimports": `printf 'goimports\n' >> "$GO_TOOL_LOG"
/bin/cat "$1"`,
		"gofumpt": "printf 'gofumpt\\n' >> \"$GO_TOOL_LOG\"\n" + gofumptCopyScript,
	})
	path := goFileUnderConfig(t, goimportsAndGofumptYML, validGoSrc)

	res := NewGo().Format(t.Context(), filepath.Dir(path), path)
	assertFormatted(t, res)
	qt.Check(t, qt.Equals(string(readSource(t, logPath)), "goimports\ngofumpt\n"))
	qt.Check(t, qt.Equals(string(readSource(t, argv)), "-extra=group_params\n"),
		qt.Commentf("gofumpt formats goimports' stdout, so it must not also be given the path"))
	qt.Check(t, qt.Equals(string(readSource(t, path)), validGoSrc))
}

func TestGoFormatterSkipsGoimportsWhenItIsNotOnPATH(t *testing.T) {
	isolateDiskCache(t)
	argv := filepath.Join(t.TempDir(), "argv")
	t.Setenv("GO_TOOL_ARGV", argv)
	writeFakeTools(t, map[string]string{
		"gofumpt": gofumptCopyScript,
	})
	path := goFileUnderConfig(t, goimportsAndGofumptYML, validGoSrc)

	res := NewGo().Format(t.Context(), filepath.Dir(path), path)
	assertFormatted(t, res)
	args := string(readSource(t, argv))
	qt.Check(t, qt.IsTrue(argvHasLine(args, path)), qt.Commentf("gofumpt reads the file when goimports is absent: %q", args))
	qt.Check(t, qt.IsTrue(argvHasLine(args, "-extra=group_params")))
}

func TestGoFormatterSkipsInvalidSyntaxWithoutCallingGofumpt(t *testing.T) {
	isolateDiskCache(t)
	logPath := filepath.Join(t.TempDir(), "log")
	t.Setenv("GO_TOOL_LOG", logPath)
	writeFakeTools(t, map[string]string{
		"gofumpt": `printf 'called\n' > "$GO_TOOL_LOG"
printf 'package changed\n'`,
	})
	src := "package main\nfunc {\n"
	path := goFileUnderConfig(t, groupParamsYML, src)

	res := NewGo().Format(t.Context(), filepath.Dir(path), path)
	qt.Check(t, qt.IsTrue(res.Skipped))
	qt.Check(t, qt.Equals(res.Diagnostic, ""))
	qt.Check(t, qt.Equals(string(readSource(t, path)), src))
	_, err := os.Stat(logPath)
	qt.Check(t, qt.IsTrue(os.IsNotExist(err)), qt.Commentf("gofumpt must not be started for invalid Go"))
}

func TestGoFormatterReportsGofumptFailure(t *testing.T) {
	isolateDiskCache(t)
	writeFakeTools(t, map[string]string{
		"gofumpt": "echo boom >&2; exit 1",
	})
	path := goFileUnderConfig(t, gofumptOnlyYML, validGoSrc)

	res := NewGo().Format(t.Context(), filepath.Dir(path), path)
	qt.Check(t, qt.IsNil(res.Err))
	qt.Check(t, qt.IsFalse(res.Skipped))
	qt.Check(t, qt.StringContains(res.Diagnostic, "boom"))
	qt.Check(t, qt.Equals(string(readSource(t, path)), validGoSrc))
}

func TestGoFormatterReportsEmptyGofumptOutput(t *testing.T) {
	isolateDiskCache(t)
	writeFakeTools(t, map[string]string{
		"gofumpt": "exit 0",
	})
	path := goFileUnderConfig(t, gofumptOnlyYML, validGoSrc)

	res := NewGo().Format(t.Context(), filepath.Dir(path), path)
	qt.Check(t, qt.StringContains(res.Diagnostic, "no output"))
	qt.Check(t, qt.Equals(string(readSource(t, path)), validGoSrc))
}

func TestGoFormatterReportsCancelledContext(t *testing.T) {
	isolateDiskCache(t)
	writeFakeTools(t, map[string]string{
		"gofumpt": `/bin/cat "$1"`,
	})
	path := goFileUnderConfig(t, gofumptOnlyYML, validGoSrc)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	res := NewGo().Format(ctx, filepath.Dir(path), path)
	qt.Check(t, qt.Equals(res.Diagnostic, context.Canceled.Error()))
	qt.Check(t, qt.Equals(string(readSource(t, path)), validGoSrc))
}

func TestParseGolangciFormatters(t *testing.T) {
	cases := []struct {
		name        string
		src         string
		goimports   bool
		groupParams bool
	}{
		{name: "empty", src: ""},
		{name: "fleet shape", src: goimportsAndGofumptYML, goimports: true, groupParams: true},
		{
			name: "group-params false",
			src: `formatters:
  enable:
    - goimports
  settings:
    gofumpt:
      extra:
        group-params: false
`,
			goimports: true,
		},
		{
			name: "commented flag",
			src: `formatters:
  settings:
    gofumpt:
      extra:
        # group-params: true
`,
		},
		{
			name: "under linters",
			src: `linters:
  enable:
    - goimports
  settings:
    gofumpt:
      extra:
        group-params: true
`,
		},
		{
			name:      "inline enable",
			src:       "formatters:\n  enable: [goimports, gofumpt]\n",
			goimports: true,
		},
		{
			name: "compact list",
			src: `formatters:
  enable:
  - goimports
`,
			goimports: true,
		},
		{
			name: "quoted name",
			src: `formatters:
  enable:
    - "goimports"
`,
			goimports: true,
		},
		{
			name:        "crlf",
			src:         "formatters:\r\n  settings:\r\n    gofumpt:\r\n      extra:\r\n        group-params: true\r\n",
			groupParams: true,
		},
		{
			name:      "document start",
			src:       "---\nformatters:\n  enable: [goimports]\n",
			goimports: true,
		},
		{
			name: "tabs skipped",
			src:  "formatters:\n\tenable:\n\t- goimports\n",
		},
		{
			name:      "hash inside quoted value",
			src:       "formatters:\n  note: \"keep # this\"\n  enable: [goimports]\n",
			goimports: true,
		},
		{
			name: "empty key",
			src:  "formatters:\n  : [goimports]\n",
		},
		{
			name: "goimports not under enable",
			src: `formatters:
  disable:
    - goimports
`,
		},
		{
			name:      "single-quoted list item",
			src:       "formatters:\n  enable:\n    - 'goimports'\n",
			goimports: true,
		},
		{
			name: "inline enable without goimports",
			src:  "formatters:\n  enable: [gofumpt]\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := parseGolangciFormatters([]byte(tc.src))
			qt.Check(t, qt.Equals(opts.goimports, tc.goimports))
			qt.Check(t, qt.Equals(opts.groupParams, tc.groupParams))
		})
	}
}

func TestGolangciGoFormatUsesNearestParent(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "mod")
	child := filepath.Join(parent, "pkg")
	qt.Assert(t, qt.IsNil(os.MkdirAll(child, 0o750)))
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(parent, ".golangci.yml"), []byte(goimportsAndGofumptYML), 0o600)))
	childYML := `version: "2"
formatters:
  enable:
    - gofumpt
  settings:
    gofumpt:
      extra:
        group-params: false
`
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(child, ".golangci.yml"), []byte(childYML), 0o600)))

	near := golangciGoFormat(filepath.Join(child, "t.go"))
	qt.Check(t, qt.IsFalse(near.goimports), qt.Commentf("the child config does not list goimports"))
	qt.Check(t, qt.IsFalse(near.groupParams), qt.Commentf("the child config turns group-params off"))

	far := golangciGoFormat(filepath.Join(parent, "t.go"))
	qt.Check(t, qt.IsTrue(far.goimports))
	qt.Check(t, qt.IsTrue(far.groupParams))
}

func TestGolangciGoFormatReadsLintConfigNames(t *testing.T) {
	const jsonCfg = `{"formatters":{"enable":["goimports"],"settings":{"gofumpt":{"extra":{"group-params":true}}}}}`
	const tomlCfg = `[formatters]
enable = ["goimports"]
[formatters.settings.gofumpt.extra]
group-params = true
`
	cases := []struct {
		name string
		body string
	}{
		{".golangci.yaml", goimportsAndGofumptYML},
		{".golangci.toml", tomlCfg},
		{".golangci.json", jsonCfg},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			pkg := filepath.Join(dir, "pkg")
			qt.Assert(t, qt.IsNil(os.Mkdir(pkg, 0o750)))
			qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, tc.name), []byte(tc.body), 0o600)))
			opts := golangciGoFormat(filepath.Join(pkg, "t.go"))
			qt.Check(t, qt.IsTrue(opts.goimports))
			qt.Check(t, qt.IsTrue(opts.groupParams))
		})
	}
}

func TestGolangciGoFormatPrefersYmlOverYaml(t *testing.T) {
	dir := t.TempDir()
	pkg := filepath.Join(dir, "pkg")
	qt.Assert(t, qt.IsNil(os.Mkdir(pkg, 0o750)))
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, ".golangci.yml"), []byte(gofumptOnlyYML), 0o600)))
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, ".golangci.yaml"), []byte(goimportsAndGofumptYML), 0o600)))
	opts := golangciGoFormat(filepath.Join(pkg, "t.go"))
	qt.Check(t, qt.IsFalse(opts.goimports))
	qt.Check(t, qt.IsFalse(opts.groupParams))
}

package main

import (
	"strings"
)

// commandDef is the single source of truth for --help, completions, and the
// man page. Hidden entries exist so `completion` / `man` cannot drift from
// the same table but do not show up in operator-facing help.
type commandDef struct {
	name     string
	complete []string
	args     string
	summary  string
	flags    []flagDef
	hidden   bool
}

type flagDef struct {
	name    string
	args    string
	summary string
}

var commands = []commandDef{
	{
		name:    "",
		summary: "read a PostToolUse payload from stdin (normal hook invocation)",
	},
	{
		name:    "--install",
		summary: "wire this binary into ~/.claude/settings.json (PostToolUse) and ~/.cursor/hooks.json (afterFileEdit)",
		flags: []flagDef{{
			name:    "--dry-run",
			summary: "preview the Claude and Cursor hook diffs, without writing",
		}},
	},
	{
		name:    "--uninstall",
		summary: "remove it from ~/.claude/settings.json and ~/.cursor/hooks.json",
		flags: []flagDef{{
			name:    "--dry-run",
			summary: "preview uninstall without writing",
		}},
	},
	{
		name:    "--upgrade",
		summary: "download and replace this platform's latest release binary, and refresh the bunx formatters",
		flags: []flagDef{{
			name:    "--dry-run",
			summary: "preview the binary upgrade without writing",
		}},
	},
	{
		name:    "--check",
		args:    "PATH...",
		summary: "report files a formatter would change, without changing them (exit 1 if any; exit 2 on usage or I/O error)",
		flags: []flagDef{
			{
				name:    "--write",
				args:    "PATH...",
				summary: "format those files in place instead of reporting them",
			},
			{
				name:    "--json",
				args:    "PATH...",
				summary: "same report as one JSON document on stdout",
			},
		},
	},
	{
		name:    "--doctor",
		summary: "report each formatter, its tools, extensions, and whether the hook is wired",
		flags: []flagDef{{
			name:    "--json",
			summary: "same report as one JSON document on stdout",
		}},
	},
	{
		name:    "--version",
		summary: "print version and build info",
	},
	{
		name:     "--help, -h",
		complete: []string{"--help", "-h"},
		summary:  "show this help",
	},
	{
		name:    "completion",
		args:    "<shell>",
		summary: "print bash, zsh, fish, or powershell completion script",
		hidden:  true,
	},
	{
		name:    "man",
		summary: "print the man page on stdout",
		hidden:  true,
	},
}

func (c commandDef) completionNames() []string {
	if len(c.complete) > 0 {
		return c.complete
	}
	if c.name == "" {
		return nil
	}
	return []string{c.name}
}

func (c commandDef) usageInvocation() string {
	parts := make([]string, 0, 3)
	if c.name != "" {
		parts = append(parts, c.name)
	}
	if c.args != "" {
		parts = append(parts, c.args)
	}
	return strings.Join(parts, " ")
}

func (f flagDef) usageInvocation(parent string) string {
	parts := []string{parent, f.name}
	if f.args != "" {
		parts = append(parts, f.args)
	}
	return strings.Join(parts, " ")
}

// usage is generated from commands so --help cannot drift from completions
// and the man page.
var usage = buildUsage()

func buildUsage() string {
	var b strings.Builder
	b.WriteString("format-dispatch is a Claude Code PostToolUse hook. Invoked with no\n")
	b.WriteString("arguments, it reads a hook payload from stdin and formats the file it names.\n\n")
	b.WriteString("Usage:\n")
	for _, c := range commands {
		if c.hidden {
			continue
		}
		writeUsageLine(&b, c.usageInvocation(), c.summary)
		for _, f := range c.flags {
			writeUsageLine(&b, f.usageInvocation(c.name), f.summary)
		}
	}
	return b.String()
}

func writeUsageLine(b *strings.Builder, invoc, summary string) {
	left := "  format-dispatch"
	if invoc != "" {
		left += " " + invoc
	}
	const width = 36
	b.WriteString(left)
	if len(left) < width {
		b.WriteString(strings.Repeat(" ", width-len(left)))
	} else {
		b.WriteString("  ")
	}
	b.WriteString(summary)
	b.WriteByte('\n')
}

func visibleCommands() []commandDef {
	out := make([]commandDef, 0, len(commands))
	for _, c := range commands {
		if c.hidden {
			continue
		}
		out = append(out, c)
	}
	return out
}

func completionWords() []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(s string) {
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, c := range visibleCommands() {
		for _, n := range c.completionNames() {
			add(n)
		}
		for _, f := range c.flags {
			add(f.name)
		}
	}
	return out
}

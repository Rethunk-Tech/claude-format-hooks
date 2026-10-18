package main

import (
	"fmt"
	"os"
	"strings"
)

var completionShells = []string{"bash", "zsh", "fish", "powershell"}

func runCompletion(args []string) int {
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "format-dispatch completion: need one shell (%s)\n", strings.Join(completionShells, ", "))
		return 1
	}
	out, err := generateCompletion(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "format-dispatch completion: %v\n", err)
		return 1
	}
	fmt.Print(out)
	return 0
}

func runMan(args []string) int {
	if !parseNoExtra(args, "man") {
		return 1
	}
	fmt.Print(generateMan())
	return 0
}

func generateCompletion(shell string) (string, error) {
	switch shell {
	case "bash":
		return generateBash(), nil
	case "zsh":
		return generateZsh(), nil
	case "fish":
		return generateFish(), nil
	case "powershell":
		return generatePowerShell(), nil
	default:
		return "", fmt.Errorf("unknown shell %q (want %s)", shell, strings.Join(completionShells, ", "))
	}
}

func generateBash() string {
	words := strings.Join(completionWords(), " ")
	return `# bash completion for format-dispatch
_format_dispatch() {
	local cur="${COMP_WORDS[COMP_CWORD]}"
	COMPREPLY=($(compgen -W '` + words + `' -- "$cur"))
}
complete -F _format_dispatch format-dispatch
`
}

func generateZsh() string {
	var b strings.Builder
	b.WriteString("#compdef format-dispatch\n\n")
	b.WriteString("_arguments -s -S \\\n")
	for _, w := range completionWords() {
		b.WriteString("  '")
		b.WriteString(zshEscape(w))
		b.WriteString("' \\\n")
	}
	b.WriteString("  '*:file:_files'\n")
	return b.String()
}

func zshEscape(s string) string {
	return strings.ReplaceAll(s, "'", `'\''`)
}

func generateFish() string {
	var b strings.Builder
	b.WriteString("complete -c format-dispatch -f\n")
	for _, c := range visibleCommands() {
		for _, n := range c.completionNames() {
			writeFishComplete(&b, n, c.summary)
		}
		for _, f := range c.flags {
			writeFishComplete(&b, f.name, f.summary)
		}
	}
	return b.String()
}

func writeFishComplete(b *strings.Builder, name, summary string) {
	b.WriteString("complete -c format-dispatch -a ")
	b.WriteString(fishQuote(name))
	if summary != "" {
		b.WriteString(" -d ")
		b.WriteString(fishQuote(summary))
	}
	b.WriteByte('\n')
}

func fishQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func generatePowerShell() string {
	var quoted []string
	for _, w := range completionWords() {
		quoted = append(quoted, "'"+strings.ReplaceAll(w, "'", "''")+"'")
	}
	return `Register-ArgumentCompleter -Native -CommandName format-dispatch -ScriptBlock {
	param($wordToComplete)
	$cmds = @(` + strings.Join(quoted, ", ") + `)
	$cmds | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
		[System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterName', $_)
	}
}
`
}

func generateMan() string {
	var b strings.Builder
	b.WriteString(`.TH FORMAT-DISPATCH 1
.SH NAME
format-dispatch \- Claude Code PostToolUse format hook
.SH SYNOPSIS
.B format-dispatch
[\fIcommand\fR]
.br
.B format-dispatch
reads a PostToolUse payload from stdin when invoked with no arguments.
.SH DESCRIPTION
format-dispatch is a Claude Code PostToolUse hook. Invoked with no
arguments, it reads a hook payload from stdin and formats the file it names.
.SH COMMANDS
`)
	for _, c := range visibleCommands() {
		invoc := c.usageInvocation()
		if invoc == "" {
			invoc = "(no arguments)"
		}
		b.WriteString(".TP\n.B ")
		b.WriteString(manEscape(invoc))
		b.WriteByte('\n')
		b.WriteString(manEscape(c.summary))
		b.WriteByte('\n')
		for _, f := range c.flags {
			b.WriteString(".TP\n.B ")
			b.WriteString(manEscape(f.usageInvocation(c.name)))
			b.WriteByte('\n')
			b.WriteString(manEscape(f.summary))
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func manEscape(s string) string {
	return strings.ReplaceAll(s, `\`, `\\`)
}

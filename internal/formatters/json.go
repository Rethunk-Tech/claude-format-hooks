package formatters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Rethunk-Tech/claude-format-hooks/internal/config"
)

// jsonFormatter reformats JSON in place, in-process.
//
// It deliberately uses json.Indent rather than Unmarshal+Marshal:
// json.Indent re-indents at the byte level without building an object
// graph, so it preserves source key order exactly. A round-trip through
// map[string]interface{} would silently alphabetize every object's keys
// (encoding/json.Marshal sorts map keys), which is not what "format"
// means for a JSON file a human or another tool authored.
type jsonFormatter struct{ cfg config.Config }

// NewJSON returns the native jsonFormatter for .json.
func NewJSON(cfg config.Config) jsonFormatter { return jsonFormatter{cfg: cfg} }

// jsonRouter sends .json to biome where the project has a biome config unless
// biome is disabled, and to the native formatter everywhere else. Its Name
// remains "json" even when Format delegates to biome, so callers applying
// project formatter opt-outs must account for the delegated formatter name.
//
// Biome's config governs .json as much as it governs .ts — indent, line width
// and `expand`, which decides whether an object collapses onto one line. The
// native formatter cannot read any of that: json.Indent always expands every
// object. Formatting a project's own biome.json with it therefore produces a
// file that project's biome formatting rejects, so the config that opted in is
// the config that gets violated. Projects without biome keep the native formatter,
// which needs no bunx and no config of its own.
type jsonRouter struct {
	biome         Formatter
	native        Formatter
	biomeDisabled bool
}

// NewJSONRouter returns the .json formatter: biome when the project has a
// biome config and biome is enabled, the dependency-free native formatter
// otherwise. Project-level "biome" disables therefore cannot be enforced by
// registry name filtering alone; hook and --check callers must preserve the
// router's delegated formatter opt-outs.
func NewJSONRouter(cfg config.Config) jsonRouter {
	return jsonRouter{
		biome:         NewBiome(),
		native:        NewJSON(cfg),
		biomeDisabled: cfg.IsFormatterDisabled("biome"),
	}
}

func (jsonRouter) Name() string { return "json" }

// Tools reports biome and bunx so --doctor does not label the router native.
// Format still falls back to encoding/json.Indent when those are absent.
func (jsonRouter) Tools() []string { return []string{"biome", "bunx"} }

func (r jsonRouter) Format(ctx context.Context, projectRoot, abs string) Result {
	// Biome exits 0 on a suffix it does not recognize (--no-errors-on-unmatched)
	// and leaves the file as it was. These copies are still JSON.
	if strings.HasSuffix(strings.ToLower(filepath.Base(abs)), ".json.example") {
		return r.native.Format(ctx, projectRoot, abs)
	}
	// Without a usable Biome launcher the native formatter is still better than
	// leaving the file untouched, even though it ignores `expand`.
	if biomeUsable(projectRoot, abs, r.biomeDisabled) {
		return r.biome.Format(ctx, projectRoot, abs)
	}
	return r.native.Format(ctx, projectRoot, abs)
}

// FormatStdin answers --check's question without a scratch copy, following the
// same delegate Format would pick for abs.
// formatStdinVia adapts a delegate held as a plain Formatter to the
// StdinFormatter a caller needs, reporting ok=false when it has no stdin mode.
func formatStdinVia(ctx context.Context, f Formatter, projectRoot, abs string, src []byte) ([]byte, Result, bool) {
	sf, ok := f.(StdinFormatter)
	if !ok {
		return nil, Result{}, false
	}
	return sf.FormatStdin(ctx, projectRoot, abs, src)
}

func (r jsonRouter) FormatStdin(ctx context.Context, projectRoot, abs string, src []byte) ([]byte, Result, bool) {
	isExample := strings.HasSuffix(strings.ToLower(filepath.Base(abs)), ".json.example")
	if !isExample && biomeUsable(projectRoot, abs, r.biomeDisabled) {
		return formatStdinVia(ctx, r.biome, projectRoot, abs, src)
	}
	return formatStdinVia(ctx, r.native, projectRoot, abs, src)
}

func (jsonFormatter) Name() string { return "json" }

func (f jsonFormatter) Format(_ context.Context, _, abs string) Result {
	src, err := os.ReadFile(filepath.Clean(abs))
	if err != nil {
		return Result{Err: fmt.Errorf("read: %w", err)}
	}
	return writeIfChanged(abs, src, f.format(src, abs), 0o644)
}

// FormatStdin formats src as JSON for abs. Only the indent resolution needs
// the path, so this answers --check's question without a scratch copy beside
// the file.
func (f jsonFormatter) FormatStdin(_ context.Context, _, abs string, src []byte) ([]byte, Result, bool) {
	return f.format(src, abs), Result{}, true
}

// format is the in-process formatting both Format and FormatStdin apply.
func (f jsonFormatter) format(src []byte, abs string) []byte {
	spec := config.ResolveJSONIndent(f.cfg, abs)
	indent := "\t"
	if !spec.UseTabs {
		indent = strings.Repeat(" ", clampedIndentWidth(spec.Size))
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, src, "", indent); err != nil {
		if jsonCNamedJSON(abs) {
			buf.Reset()
			if err := indentJSONC(&buf, src, "", indent); err == nil {
				return append(bytes.TrimRight(buf.Bytes(), "\n\t \r"), '\n')
			}
		}
		// Comments or malformed JSON must not surface as a fixer failure;
		// the hook would put stderr in the model's context.
		return src
	}

	// json.Indent passes through any insignificant whitespace already
	// trailing the top-level value in src verbatim (it does not strip
	// it). Trim before appending exactly one newline, or a file that
	// already ends in "}\n" would gain an extra blank line every single
	// time it's formatted — a non-idempotent, ever-growing bug.
	return append(bytes.TrimRight(buf.Bytes(), "\n\t \r"), '\n')
}

// maxIndentWidth caps spaces taken from untrusted config / .editorconfig.
// strings.Repeat and shfmt's Indent would otherwise accept a multi-gigabyte
// width from a hostile indentSize.
const maxIndentWidth = 32

// jsonCNamedJSON is the editor JSONC set that still uses a .json suffix.
func jsonCNamedJSON(abs string) bool {
	base := strings.ToLower(filepath.Base(abs))
	if !strings.HasSuffix(base, ".json") {
		return false
	}
	if strings.HasPrefix(base, "tsconfig") || strings.HasPrefix(base, "jsconfig") {
		return true
	}
	return strings.EqualFold(filepath.Base(filepath.Dir(abs)), ".vscode")
}

type jtok struct {
	kind  rune
	raw   []byte
	trail bool
}

func indentJSONC(dst *bytes.Buffer, src []byte, prefix, indent string) error {
	toks, err := lexJSONC(src)
	if err != nil {
		return err
	}
	p := jsoncPrinter{toks: toks, dst: dst, prefix: prefix, indent: indent, atLine: true}
	if err := p.value(); err != nil {
		return err
	}
	p.emitComments(false)
	if p.i != len(p.toks) {
		return fmt.Errorf("invalid character after top-level value")
	}
	return nil
}

func lexJSONC(src []byte) ([]jtok, error) {
	var toks []jtok
	i := 0
	if len(src) >= 3 && src[0] == 0xef && src[1] == 0xbb && src[2] == 0xbf {
		i = 3
	}
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '/' && i+1 < len(src) && (src[i+1] == '/' || src[i+1] == '*'):
			tok, next, err := lexJSONCComment(src, i)
			if err != nil {
				return nil, err
			}
			toks = append(toks, tok)
			i = next
		case c == '"':
			j, err := scanJSONCString(src, i)
			if err != nil {
				return nil, err
			}
			toks = append(toks, jtok{kind: 'v', raw: src[i:j]})
			i = j
		case c == '{' || c == '}' || c == '[' || c == ']' || c == ':' || c == ',':
			toks = append(toks, jtok{kind: rune(c), raw: src[i : i+1]})
			i++
		default:
			j, err := scanJSONCLiteral(src, i)
			if err != nil {
				return nil, err
			}
			toks = append(toks, jtok{kind: 'v', raw: src[i:j]})
			i = j
		}
	}
	return toks, nil
}

func lexJSONCComment(src []byte, i int) (jtok, int, error) {
	trail := commentIsTrailing(src, i)
	if src[i+1] == '/' {
		j := i + 2
		for j < len(src) && src[j] != '\n' {
			j++
		}
		raw := src[i:j]
		if n := len(raw); n > 0 && raw[n-1] == '\r' {
			raw = raw[:n-1]
		}
		return jtok{kind: '/', raw: raw, trail: trail}, j, nil
	}
	j := i + 2
	for j+1 < len(src) && (src[j] != '*' || src[j+1] != '/') {
		j++
	}
	if j+1 >= len(src) {
		return jtok{}, 0, fmt.Errorf("unterminated comment")
	}
	j += 2
	return jtok{kind: '*', raw: src[i:j], trail: trail}, j, nil
}

func commentIsTrailing(src []byte, i int) bool {
	i--
	for i >= 0 && (src[i] == ' ' || src[i] == '\t') {
		i--
	}
	return i >= 0 && src[i] != '\n' && src[i] != '\r'
}

func scanJSONCString(src []byte, i int) (int, error) {
	i++
	for i < len(src) {
		switch src[i] {
		case '"':
			return i + 1, nil
		case '\\':
			i++
			if i >= len(src) {
				return 0, fmt.Errorf("unexpected end of JSON input")
			}
		}
		i++
	}
	return 0, fmt.Errorf("unexpected end of JSON input")
}

func scanJSONCLiteral(src []byte, i int) (int, error) {
	if src[i] == 't' || src[i] == 'f' || src[i] == 'n' {
		return scanJSONCKeyword(src, i)
	}
	return scanJSONCNumber(src, i)
}

func scanJSONCKeyword(src []byte, i int) (int, error) {
	start := i
	for i < len(src) && src[i] >= 'a' && src[i] <= 'z' {
		i++
	}
	switch string(src[start:i]) {
	case "true", "false", "null":
		return i, nil
	default:
		return 0, fmt.Errorf("invalid character %q looking for beginning of value", src[start])
	}
}

func scanJSONCNumber(src []byte, i int) (int, error) {
	start := i
	if src[i] == '-' {
		i++
		if i >= len(src) {
			return 0, fmt.Errorf("unexpected end of JSON input")
		}
	}
	if i >= len(src) || src[i] < '0' || src[i] > '9' {
		return 0, fmt.Errorf("invalid character %q looking for beginning of value", src[start])
	}
	if src[i] == '0' {
		i++
	} else {
		i = scanDigits(src, i)
	}
	var err error
	if i, err = scanJSONCFraction(src, i); err != nil {
		return 0, err
	}
	return scanJSONCExponent(src, i)
}

func scanDigits(src []byte, i int) int {
	for i < len(src) && src[i] >= '0' && src[i] <= '9' {
		i++
	}
	return i
}

func scanJSONCFraction(src []byte, i int) (int, error) {
	if i >= len(src) || src[i] != '.' {
		return i, nil
	}
	i++
	if i >= len(src) || src[i] < '0' || src[i] > '9' {
		return 0, fmt.Errorf("invalid character in number")
	}
	return scanDigits(src, i), nil
}

func scanJSONCExponent(src []byte, i int) (int, error) {
	if i >= len(src) || (src[i] != 'e' && src[i] != 'E') {
		return i, nil
	}
	i++
	if i < len(src) && (src[i] == '+' || src[i] == '-') {
		i++
	}
	if i >= len(src) || src[i] < '0' || src[i] > '9' {
		return 0, fmt.Errorf("invalid character in number")
	}
	return scanDigits(src, i), nil
}

type jsoncPrinter struct {
	toks           []jtok
	i              int
	dst            *bytes.Buffer
	prefix, indent string
	depth          int
	atLine         bool
}

func (p *jsoncPrinter) peek() rune {
	for j := p.i; j < len(p.toks); j++ {
		k := p.toks[j].kind
		if k != '/' && k != '*' {
			return k
		}
	}
	return 0
}

func (p *jsoncPrinter) take(kind rune) error {
	p.emitComments(true)
	if p.i >= len(p.toks) || p.toks[p.i].kind != kind {
		return fmt.Errorf("unexpected end of JSON input")
	}
	p.i++
	return nil
}

func (p *jsoncPrinter) writeIndent() {
	if !p.atLine {
		return
	}
	p.dst.WriteString(p.prefix)
	for range p.depth {
		p.dst.WriteString(p.indent)
	}
	p.atLine = false
}

func (p *jsoncPrinter) nl() {
	p.dst.WriteByte('\n')
	p.atLine = true
}

func (p *jsoncPrinter) emitComments(trailOK bool) {
	for p.i < len(p.toks) {
		t := p.toks[p.i]
		if t.kind != '/' && t.kind != '*' {
			return
		}
		p.i++
		if trailOK && t.trail && !p.atLine {
			p.dst.WriteByte(' ')
			p.dst.Write(t.raw)
		} else {
			if !p.atLine {
				p.nl()
			}
			p.writeIndent()
			p.dst.Write(t.raw)
		}
		if t.kind == '/' || bytes.ContainsRune(t.raw, '\n') {
			p.nl()
		}
	}
}

func (p *jsoncPrinter) value() error {
	p.emitComments(false)
	if p.i >= len(p.toks) {
		return fmt.Errorf("unexpected end of JSON input")
	}
	switch p.toks[p.i].kind {
	case '{':
		p.i++
		return p.container('}', true)
	case '[':
		p.i++
		return p.container(']', false)
	case 'v':
		p.writeIndent()
		p.dst.Write(p.toks[p.i].raw)
		p.i++
		p.emitComments(true)
		return nil
	default:
		return fmt.Errorf("invalid character %q looking for beginning of value", p.toks[p.i].kind)
	}
}

func (p *jsoncPrinter) container(closer rune, object bool) error {
	p.writeIndent()
	if closer == '}' {
		p.dst.WriteByte('{')
	} else {
		p.dst.WriteByte('[')
	}
	if p.peek() == closer && !p.hasCommentBefore() {
		_ = p.take(closer)
		p.dst.WriteRune(closer)
		p.emitComments(true)
		return nil
	}
	p.nl()
	p.depth++
	for {
		p.emitComments(false)
		if p.peek() == closer {
			break
		}
		if object {
			if err := p.key(); err != nil {
				return err
			}
			if err := p.take(':'); err != nil {
				return err
			}
			p.dst.WriteString(": ")
			p.atLine = false
		}
		if err := p.value(); err != nil {
			return err
		}
		if p.peek() == ',' {
			if err := p.take(','); err != nil {
				return err
			}
			p.dst.WriteByte(',')
			p.emitComments(true)
			if p.peek() == closer {
				break
			}
			if !p.atLine {
				p.nl()
			}
			continue
		}
		break
	}
	if !p.atLine {
		p.nl()
	}
	p.emitComments(false)
	p.depth--
	p.writeIndent()
	if err := p.take(closer); err != nil {
		return err
	}
	p.dst.WriteRune(closer)
	p.emitComments(true)
	return nil
}

func (p *jsoncPrinter) hasCommentBefore() bool {
	return p.i < len(p.toks) && (p.toks[p.i].kind == '/' || p.toks[p.i].kind == '*')
}

func (p *jsoncPrinter) key() error {
	p.emitComments(false)
	if p.i >= len(p.toks) || p.toks[p.i].kind != 'v' {
		return fmt.Errorf("invalid character looking for beginning of object key string")
	}
	p.writeIndent()
	p.dst.Write(p.toks[p.i].raw)
	p.i++
	p.emitComments(true)
	return nil
}

func clampedIndentWidth(size int) int {
	if size < 1 {
		return 1
	}
	if size > maxIndentWidth {
		return maxIndentWidth
	}
	return size
}

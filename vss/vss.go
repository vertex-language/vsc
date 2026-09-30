// Package vss reads .vss files: a package's styles, in standard CSS after
// a header of a `package` line and `import` lines (proposed_vsx.md §7).
//
//	/* ui/kit/button.vss */
//	package kit
//
//	import "ui/theme"
//
//	.button { padding: 6px 12px; &:hover { color: red } }
//
// A package's .vss files become one stylesheet, which the package carries
// in its binary as a generated declaration:
//
//	public let __vssSheet = component.Sheet(package: "kit", css: "…", after: [theme.__vssSheet])
//
// The CSS is the package's own cascade layer, and inside it the package's
// scope: its rules reach the elements its own markup makes, and stop at
// another package's. Rules on the document -- :root, html, body -- are in
// the layer and not the scope.
//
//	@layer kit {
//	  :root { --kit-radius: 6px }
//	  @scope ([data-p="kit"]) to ([data-p]:not([data-p="kit"])) {
//	    .button { … }
//	  }
//	}
//
// `after` is the sheets of the packages this one imports that have styles
// of their own: the runtime puts theirs first, so a package overrides what
// it builds on, and the program overrides everything. The engine (web)
// reads the CSS; nothing here parses more of it than its top-level rules.
package vss

import (
	"fmt"
	"sort"
	"strings"

	"github.com/vertex-language/vsc/token"
)

// Extension is a style file's extension.
const Extension = ".vss"

// SheetName is the declaration a styled package's generated source makes.
const SheetName = "__vssSheet"

// A File is one .vss file, its header read.
type File struct {
	Unit    *token.File
	Package string
	Imports []Import
	// Body is the CSS after the header.
	Body string
	// BodyOffset is where Body starts in the file.
	BodyOffset int
}

// An Import is an `import "path"` line of a .vss file.
type Import struct {
	Path string
	Pos  token.Pos
}

// Parse reads a .vss file's header: comments and blank lines, then
// `package name`, then any number of `import "path"` lines (or one
// `import ( "a" "b" )` group). The rest is CSS.
func Parse(unit *token.File) (*File, []token.Diagnostic) {
	src := unit.Text()
	f := &File{Unit: unit}
	var diags []token.Diagnostic
	errAt := func(off, end int, msg string) {
		diags = append(diags, token.Diagnostic{Pos: unit.Pos(off), End: unit.Pos(end), Severity: token.Error, Message: msg, File: unit})
	}
	i := skipTrivia(src, 0)
	word, end := identAt(src, i)
	if word != "package" {
		errAt(i, i, "a .vss file begins with its package: `package name`")
		return f, diags
	}
	i = skipSpaces(src, end)
	name, end := identAt(src, i)
	if name == "" {
		errAt(i, i, "expected the package's name after `package`")
		return f, diags
	}
	f.Package = name
	i = end
	for {
		j := skipTrivia(src, i)
		word, end := identAt(src, j)
		if word != "import" {
			i = j
			break
		}
		k := skipSpaces(src, end)
		if k < len(src) && src[k] == '(' {
			k++
			for {
				k = skipTrivia(src, k)
				if k < len(src) && src[k] == ')' {
					k++
					break
				}
				path, next, ok := quotedAt(src, k)
				if !ok {
					errAt(k, k, "expected an import path, or ')'")
					return f, diags
				}
				f.Imports = append(f.Imports, Import{Path: path, Pos: unit.Pos(k)})
				k = next
			}
			i = k
			continue
		}
		path, next, ok := quotedAt(src, k)
		if !ok {
			errAt(k, k, "expected an import path: `import \"ui/kit\"`")
			return f, diags
		}
		f.Imports = append(f.Imports, Import{Path: path, Pos: unit.Pos(k)})
		i = next
	}
	f.Body = string(src[i:])
	f.BodyOffset = i
	diags = append(diags, checkBody(f)...)
	return f, diags
}

// checkBody reports what a stylesheet cannot mean: braces that do not
// balance, and a string or comment left open.
func checkBody(f *File) []token.Diagnostic {
	var diags []token.Diagnostic
	b := f.Body
	var opens []int
	for i := 0; i < len(b); i++ {
		switch c := b[i]; {
		case c == '/' && i+1 < len(b) && b[i+1] == '*':
			end := strings.Index(b[i+2:], "*/")
			if end < 0 {
				diags = append(diags, diag(f, i, "unterminated comment"))
				return diags
			}
			i += end + 3
		case c == '"' || c == '\'':
			j := i + 1
			for j < len(b) && b[j] != c && b[j] != '\n' {
				if b[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(b) || b[j] != c {
				diags = append(diags, diag(f, i, "unterminated string"))
				return diags
			}
			i = j
		case c == '{':
			opens = append(opens, i)
		case c == '}':
			if len(opens) == 0 {
				diags = append(diags, diag(f, i, "'}' closes nothing"))
				continue
			}
			opens = opens[:len(opens)-1]
		}
	}
	for _, o := range opens {
		diags = append(diags, diag(f, o, "'{' is never closed"))
	}
	return diags
}

func diag(f *File, off int, msg string) token.Diagnostic {
	p := f.Unit.Pos(f.BodyOffset + off)
	return token.Diagnostic{Pos: p, End: p, Severity: token.Error, Message: msg, File: f.Unit}
}

// Emit is a package's stylesheet: its files' CSS, in file order, as its
// cascade layer, scoped to its elements. @import, @font-face and
// @property are the document's and stay outside the layer.
func Emit(pkg string, files []*File) string {
	var global, document, scoped []string
	for _, f := range files {
		for _, item := range topLevel(f.Body) {
			switch {
			case item.at == "import" || item.at == "font-face" || item.at == "property" || item.at == "charset":
				global = append(global, item.text)
			case item.at == "" && documentLevel(item.prelude):
				document = append(document, item.text)
			default:
				scoped = append(scoped, item.text)
			}
		}
	}
	var b strings.Builder
	for _, g := range global {
		b.WriteString(g)
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "@layer %s {\n", pkg)
	for _, d := range document {
		b.WriteString(d)
		b.WriteString("\n")
	}
	if len(scoped) > 0 {
		fmt.Fprintf(&b, "@scope ([data-p=%q]) to ([data-p]:not([data-p=%q])) {\n", pkg, pkg)
		for _, s := range scoped {
			b.WriteString(s)
			b.WriteString("\n")
		}
		b.WriteString("}\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// Source is the generated Vertex a styled package carries: its sheet, and
// the sheets it comes after -- those of the packages it imports that have
// styles, by import path.
func Source(pkg, css string, after []string, tokens []Token) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by vsc from package %s's .vss files. Do not edit.\n", pkg)
	fmt.Fprintf(&b, "package %s\n\n", pkg)
	b.WriteString("import __vss_component \"ui/component\"\n")
	sorted := append([]string(nil), after...)
	sort.Strings(sorted)
	for i, path := range sorted {
		fmt.Fprintf(&b, "import __vss_after%d %q\n", i, path)
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "public let %s = __vss_component.Sheet(package: %q, css: %s, after: [", SheetName, pkg, quote(css))
	for i := range sorted {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "__vss_after%d.%s", i, SheetName)
	}
	b.WriteString("])\n")
	// A library's tokens, by name: `kit.Tokens.Accent`. Package main has
	// no one to publish them to.
	if pkg != "main" && len(tokens) > 0 {
		b.WriteString("\n/// The package's style tokens: the custom properties its .vss files\n")
		b.WriteString("/// register with @property, which a program sets to theme it.\n")
		b.WriteString("public enum Tokens {\n")
		for _, t := range tokens {
			fmt.Fprintf(&b, "    /// %s: %s\n", t.Name, t.Syntax)
			fmt.Fprintf(&b, "    public static let %s = __vss_component.Token(name: %s, syntax: %s)\n", t.Member(pkg), quote(t.Name), quote(t.Syntax))
		}
		b.WriteString("}\n")
	}
	return b.String()
}

// A Token is a custom property a package registers with @property.
type Token struct {
	Name   string // --kit-accent
	Syntax string // <color>
}

// Member is the token's name in the package's Tokens: --kit-text-color
// in package kit is TextColor.
func (t Token) Member(pkg string) string {
	rest := strings.TrimPrefix(strings.TrimPrefix(t.Name, "--"), pkg+"-")
	var b strings.Builder
	upper := true
	for _, r := range rest {
		if r == '-' || r == '_' {
			upper = true
			continue
		}
		if upper && r >= 'a' && r <= 'z' {
			r -= 'a' - 'A'
		}
		upper = false
		b.WriteRune(r)
	}
	return b.String()
}

// Registered are the tokens a package's files register with @property,
// in the order they are written.
func Registered(files []*File) []Token {
	var out []Token
	for _, f := range files {
		for _, it := range topLevel(f.Body) {
			if it.at != "property" {
				continue
			}
			t := Token{Name: strings.TrimSpace(strings.TrimPrefix(it.prelude, "@property")), Syntax: "*"}
			if k := strings.Index(it.text, "syntax"); k >= 0 {
				rest := it.text[k+len("syntax"):]
				if q := strings.IndexByte(rest, '"'); q >= 0 {
					if e := strings.IndexByte(rest[q+1:], '"'); e >= 0 {
						t.Syntax = rest[q+1 : q+1+e]
					}
				}
			}
			out = append(out, t)
		}
	}
	return out
}

// An item is one top-level rule or at-rule of a stylesheet.
type item struct {
	at      string // the at-rule's name, lowercase, or "" for a style rule
	prelude string // what comes before its block or ';'
	text    string // all of it
}

// topLevel splits a stylesheet into its top-level rules and at-rules,
// keeping strings and comments whole.
func topLevel(css string) []item {
	var out []item
	i := 0
	for i < len(css) {
		// Leading space and comments belong to nothing.
		for i < len(css) {
			if css[i] == ' ' || css[i] == '\t' || css[i] == '\n' || css[i] == '\r' {
				i++
			} else if strings.HasPrefix(css[i:], "/*") {
				end := strings.Index(css[i+2:], "*/")
				if end < 0 {
					return out
				}
				i += end + 4
			} else {
				break
			}
		}
		if i >= len(css) {
			break
		}
		start := i
		depth := 0
		preludeEnd := -1
		for i < len(css) {
			c := css[i]
			if c == '/' && i+1 < len(css) && css[i+1] == '*' {
				end := strings.Index(css[i+2:], "*/")
				if end < 0 {
					i = len(css)
					break
				}
				i += end + 4
				continue
			}
			if c == '"' || c == '\'' {
				i++
				for i < len(css) && css[i] != c {
					if css[i] == '\\' {
						i++
					}
					i++
				}
				i++
				continue
			}
			if c == '{' {
				if depth == 0 && preludeEnd < 0 {
					preludeEnd = i
				}
				depth++
			}
			if c == '}' {
				depth--
				if depth == 0 {
					i++
					break
				}
			}
			if c == ';' && depth == 0 {
				preludeEnd = i
				i++
				break
			}
			i++
		}
		if preludeEnd < 0 {
			preludeEnd = i
		}
		it := item{prelude: strings.TrimSpace(css[start:min(preludeEnd, len(css))]), text: strings.TrimSpace(css[start:min(i, len(css))])}
		if strings.HasPrefix(it.prelude, "@") {
			name := it.prelude[1:]
			if k := strings.IndexAny(name, " \t\n({;"); k >= 0 {
				name = name[:k]
			}
			it.at = strings.ToLower(name)
		}
		if it.text != "" {
			out = append(out, it)
		}
	}
	return out
}

// documentLevel reports whether every selector of a rule is on the
// document itself: :root, html or body, and what is under them written
// from there (`body.dark .x` is not; `:root` and `html, body` are).
func documentLevel(prelude string) bool {
	for _, sel := range strings.Split(prelude, ",") {
		sel = strings.TrimSpace(sel)
		// One compound: `body.dark` is on the document, `body.dark .x`
		// on what is in it.
		if strings.ContainsAny(sel, " \t\n>+~") {
			return false
		}
		switch {
		case sel == ":root", sel == "html", sel == "body",
			strings.HasPrefix(sel, ":root:"), strings.HasPrefix(sel, ":root["),
			strings.HasPrefix(sel, "html:"), strings.HasPrefix(sel, "html["), strings.HasPrefix(sel, "html."),
			strings.HasPrefix(sel, "body:"), strings.HasPrefix(sel, "body["), strings.HasPrefix(sel, "body."):
		default:
			return false
		}
	}
	return prelude != ""
}

// ---- the header's tokens ----

func skipSpaces(src []byte, i int) int {
	for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
		i++
	}
	return i
}

// skipTrivia skips white space, /* */ comments and // comments: a header
// is read as Vertex is, and a /* */ comment is CSS too.
func skipTrivia(src []byte, i int) int {
	for i < len(src) {
		switch {
		case src[i] == ' ' || src[i] == '\t' || src[i] == '\n' || src[i] == '\r':
			i++
		case i+1 < len(src) && src[i] == '/' && src[i+1] == '*':
			end := strings.Index(string(src[i+2:]), "*/")
			if end < 0 {
				return i // left for the body's check to report
			}
			i += end + 4
		case i+1 < len(src) && src[i] == '/' && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		default:
			return i
		}
	}
	return i
}

func identAt(src []byte, i int) (string, int) {
	j := i
	for j < len(src) && (src[j] == '_' || src[j] >= 'a' && src[j] <= 'z' || src[j] >= 'A' && src[j] <= 'Z' || j > i && src[j] >= '0' && src[j] <= '9') {
		j++
	}
	return string(src[i:j]), j
}

func quotedAt(src []byte, i int) (string, int, bool) {
	if i >= len(src) || src[i] != '"' {
		return "", i, false
	}
	j := i + 1
	for j < len(src) && src[j] != '"' && src[j] != '\n' {
		j++
	}
	if j >= len(src) || src[j] != '"' {
		return "", i, false
	}
	return string(src[i+1 : j]), j + 1, true
}

// quote is s as a Vertex string literal: backslashes, quotes and line
// breaks escaped, so nothing in the CSS reads as an interpolation.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

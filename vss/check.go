package vss

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"

	"github.com/vertex-language/vsc/token"
)

// The property names the engine reads, as web/css publishes them
// (web/cmd/css-names prints this file; properties_test.go checks that it
// still says what the engine does).
//
//go:embed properties.json
var propertiesJSON []byte

var applied, unapplied = func() (map[string]bool, map[string]bool) {
	var lists struct {
		Applied   []string `json:"applied"`
		Unapplied []string `json:"unapplied"`
	}
	if err := json.Unmarshal(propertiesJSON, &lists); err != nil {
		panic("vss: properties.json: " + err.Error())
	}
	a, u := map[string]bool{}, map[string]bool{}
	for _, n := range lists.Applied {
		a[n] = true
	}
	for _, n := range lists.Unapplied {
		u[n] = true
	}
	return a, u
}()

// Check reports what a package's .vss file says that the engine would
// not do: a property it does not know (an error, with the nearest it
// does), one it knows but does not apply yet (a warning), and, in a
// package other than main, a rule on the document that sets anything but
// custom properties -- only the program styles the page.
//
// tokens are the custom properties each styled package the file can see
// declares, by package name: its own package's (every one it sets) and
// those of the packages it imports (what they export, see Tokens). A
// custom property named for one of them -- `--kit-accent` -- read or set
// must be one it declares. A library's @property must be named for it.
func Check(f *File, main bool, tokens map[string]map[string]bool) []token.Diagnostic {
	c := &checker{f: f, main: main, tokens: tokens}
	c.block(f.Body, 0, kindRules, false)
	return c.diags
}

// Tokens are the custom properties a package's files export: those it
// registers with @property, and those its rules on the document set.
func Tokens(files []*File) map[string]bool {
	out := map[string]bool{}
	for _, f := range files {
		for _, it := range topLevel(f.Body) {
			if it.at == "property" {
				out[strings.TrimSpace(strings.TrimPrefix(it.prelude, "@property"))] = true
			} else if it.at == "" && documentLevel(it.prelude) {
				for _, n := range customNames(it.text) {
					out[n] = true
				}
			}
		}
	}
	return out
}

// Declared are all the custom properties a package's files set anywhere,
// and register: what its own styles may read.
func Declared(files []*File) map[string]bool {
	out := Tokens(files)
	for _, f := range files {
		for _, n := range customNames(f.Body) {
			out[n] = true
		}
	}
	return out
}

// customNames are the custom properties a text declares: `--name:`.
func customNames(text string) []string {
	var out []string
	for i := 0; i+2 < len(text); i++ {
		if text[i] != '-' || text[i+1] != '-' || (i > 0 && isNameByte(text[i-1])) {
			continue
		}
		j := i + 2
		for j < len(text) && isNameByte(text[j]) {
			j++
		}
		k := j
		for k < len(text) && (text[k] == ' ' || text[k] == '\t') {
			k++
		}
		if k < len(text) && text[k] == ':' && j > i+2 {
			out = append(out, text[i:j])
		}
		i = j
	}
	return out
}

func isNameByte(c byte) bool {
	return c == '-' || c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c >= 0x80
}

type blockKind int

const (
	kindRules        blockKind = iota // a sheet, @media, @layer: rules
	kindDeclarations                  // a style rule's body: declarations and nested rules
	kindDescriptors                   // @font-face, @property: descriptors, not properties
	kindOpaque                        // @keyframes and the rest: not checked
)

type checker struct {
	f      *File
	main   bool
	tokens map[string]map[string]bool
	diags  []token.Diagnostic
}

func (c *checker) report(sev token.Severity, off int, msg string) {
	p := c.f.Unit.Pos(c.f.BodyOffset + off)
	c.diags = append(c.diags, token.Diagnostic{Pos: p, End: p, Severity: sev, Message: msg, File: c.f.Unit})
}

// block walks the statements of a block of text starting at base: rules,
// at-rules and declarations. document is whether the block is a rule on
// the document (:root, html, body).
func (c *checker) block(text string, base int, kind blockKind, document bool) {
	i := 0
	for i < len(text) {
		i = skipSpaceComments(text, i)
		if i >= len(text) {
			return
		}
		start := i
		end, stop := statementEnd(text, i)
		head := strings.TrimSpace(text[start:end])
		if stop == '{' {
			close := matchingBrace(text, end)
			inner := text[end+1 : close]
			switch {
			case kind == kindOpaque || kind == kindDescriptors:
				// Inside @keyframes and the like: not checked.
			case strings.HasPrefix(strings.ToLower(head), "@property"):
				c.property(strings.TrimSpace(head[len("@property"):]), base+start)
			case strings.HasPrefix(head, "@"):
				c.block(inner, base+end+1, atRuleKind(head, kind), document)
			default:
				c.block(inner, base+end+1, kindDeclarations, document || (kind == kindRules && documentLevel(head)))
			}
			i = close + 1
			continue
		}
		if head != "" && kind == kindDeclarations && !strings.HasPrefix(head, "@") {
			c.declaration(head, base+start, document)
		}
		i = end + 1
	}
}

func atRuleKind(head string, outer blockKind) blockKind {
	name := strings.ToLower(strings.TrimPrefix(head, "@"))
	if k := strings.IndexAny(name, " \t\n({"); k >= 0 {
		name = name[:k]
	}
	switch name {
	case "media", "supports", "layer", "scope", "container":
		// Inside a rule (nested) its body is declarations; at the top, rules.
		if outer == kindDeclarations {
			return kindDeclarations
		}
		return kindRules
	case "font-face", "property", "page", "counter-style", "font-feature-values":
		return kindDescriptors
	}
	return kindOpaque
}

// declaration checks one `name: value`.
func (c *checker) declaration(text string, off int, document bool) {
	colon := strings.IndexByte(text, ':')
	if colon < 0 {
		c.report(token.Error, off, "expected a declaration, `property: value`")
		return
	}
	name := strings.ToLower(strings.TrimSpace(text[:colon]))
	c.references(text[colon+1:], off+colon+1)
	if strings.HasPrefix(name, "--") {
		// A custom property: any value. Named for another package, it is
		// that package's token, which has to exist.
		c.token(strings.TrimSpace(text[:colon]), off, false)
		return
	}
	if document && !c.main {
		c.report(token.Error, off, "only package main styles the document; a library may set custom properties here, not '"+name+"'")
		return
	}
	switch {
	case applied[name]:
	case unapplied[name]:
		c.report(token.Warn, off, "the engine does not apply '"+name+"' yet: it will have no effect")
	default:
		msg := "unknown property '" + name + "'"
		if s := nearest(name); s != "" {
			msg += "; did you mean '" + s + "'?"
		}
		c.report(token.Error, off, msg)
	}
}

// property checks an @property's name: a library's are named for it.
func (c *checker) property(name string, off int) {
	pkg := c.f.Package
	if !strings.HasPrefix(name, "--") {
		c.report(token.Error, off, "@property registers a custom property: its name begins with --")
		return
	}
	if !c.main && !strings.HasPrefix(name, "--"+pkg+"-") {
		c.report(token.Error, off, "a token of package "+pkg+" is named --"+pkg+"-…: custom properties share the page, so a library's carry its name ('"+name+"')")
	}
}

// references checks each var(--name) in a value.
func (c *checker) references(value string, off int) {
	for i := 0; ; {
		k := strings.Index(value[i:], "var(")
		if k < 0 {
			return
		}
		j := i + k + 4
		for j < len(value) && (value[j] == ' ' || value[j] == '\t') {
			j++
		}
		e := j
		for e < len(value) && isNameByte(value[e]) {
			e++
		}
		if strings.HasPrefix(value[j:e], "--") {
			c.token(value[j:e], off+j, true)
		}
		i = e
	}
}

// token checks a custom property named for a styled package the file can
// see: --kit-accent must be one package kit declares.
func (c *checker) token(name string, off int, read bool) {
	rest := strings.TrimPrefix(name, "--")
	dash := strings.IndexByte(rest, '-')
	if dash <= 0 {
		return
	}
	pkg := rest[:dash]
	known, ok := c.tokens[pkg]
	if !ok || known[name] {
		return
	}
	// A package's own styles may set a property of its own anywhere; what
	// they read, and what another package reads or sets, has to be there.
	if pkg == c.f.Package && !read {
		return
	}
	msg := "package " + pkg + " declares no token " + name
	var names []string
	for n := range known {
		names = append(names, n)
	}
	sort.Strings(names)
	best, bestD := "", 4
	for _, n := range names {
		if d := distance(name, n); d < bestD {
			best, bestD = n, d
		}
	}
	if best != "" {
		msg += "; did you mean " + best + "?"
	}
	c.report(token.Error, off, msg)
}

// statementEnd is where the statement at i ends -- its '{', ';' or the
// block's end -- and which of those it is (0 at the end).
func statementEnd(text string, i int) (int, byte) {
	depth := 0
	for i < len(text) {
		switch c := text[i]; {
		case c == '/' && i+1 < len(text) && text[i+1] == '*':
			end := strings.Index(text[i+2:], "*/")
			if end < 0 {
				return len(text), 0
			}
			i += end + 4
			continue
		case c == '"' || c == '\'':
			i++
			for i < len(text) && text[i] != c {
				if text[i] == '\\' {
					i++
				}
				i++
			}
		case c == '(':
			depth++
		case c == ')':
			depth--
		case depth == 0 && (c == '{' || c == ';'):
			return i, c
		case depth == 0 && c == '}':
			return i, '}'
		}
		i++
	}
	return len(text), 0
}

// matchingBrace is the '}' closing the '{' at open.
func matchingBrace(text string, open int) int {
	depth := 0
	for i := open; i < len(text); i++ {
		switch c := text[i]; {
		case c == '/' && i+1 < len(text) && text[i+1] == '*':
			end := strings.Index(text[i+2:], "*/")
			if end < 0 {
				return len(text) - 1
			}
			i += end + 3
		case c == '"' || c == '\'':
			i++
			for i < len(text) && text[i] != c {
				if text[i] == '\\' {
					i++
				}
				i++
			}
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(text) - 1
}

func skipSpaceComments(text string, i int) int {
	for i < len(text) {
		switch {
		case text[i] == ' ' || text[i] == '\t' || text[i] == '\n' || text[i] == '\r' || text[i] == ';':
			i++
		case strings.HasPrefix(text[i:], "/*"):
			end := strings.Index(text[i+2:], "*/")
			if end < 0 {
				return len(text)
			}
			i += end + 4
		default:
			return i
		}
	}
	return i
}

// nearest is the applied property name nearest to name, where one is
// near enough to be what was meant.
func nearest(name string) string {
	var names []string
	for n := range applied {
		names = append(names, n)
	}
	sort.Strings(names)
	limit := min(max(len(name)/3, 1), 3)
	best, bestD := "", limit+1
	for _, n := range names {
		if d := distance(name, n); d < bestD {
			best, bestD = n, d
		}
	}
	return best
}

// distance counts insertions, deletions, changes and swaps of neighbours.
func distance(a, b string) int {
	d := make([][]int, len(a)+1)
	for i := range d {
		d[i] = make([]int, len(b)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(a)][len(b)]
}

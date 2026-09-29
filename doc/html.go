package doc

import (
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"
)

// HTML writes the package's documentation as one self-contained page, laid
// out as godoc lays out a Go package's: the overview, an index, then each
// declaration's signature with its comment. Names of the package's own
// types and functions link to where they are documented.
func HTML(w io.Writer, p *Package) error {
	h := &htmlWriter{w: w, links: map[string]string{}}
	for _, t := range p.Types {
		h.links[t.Name] = t.ID
	}
	for _, f := range p.Funcs {
		if _, ok := h.links[f.Name]; !ok {
			h.links[f.Name] = f.ID
		}
	}

	title := "package " + p.Name
	h.printf("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	h.printf("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	h.printf("<title>%s</title>\n<style>%s</style>\n</head>\n<body>\n<main>\n", esc(p.Name+" — Vertex package"), css)
	h.printf("<header><h1>%s</h1>\n", esc(title))
	if p.ImportPath != "" {
		h.printf("<pre class=\"import\">%s</pre>\n", h.code(`import "`+p.ImportPath+`"`))
	}
	h.printf("</header>\n")

	if p.Doc != "" {
		h.printf("<section id=\"overview\">\n<h2>Overview</h2>\n")
		h.comment(p.Doc)
		h.printf("</section>\n")
	}

	h.printf("<section id=\"index\">\n<h2>Index</h2>\n<ul class=\"index\">\n")
	if len(p.Consts) > 0 {
		h.printf("<li><a href=\"#constants\">Constants</a></li>\n")
	}
	if len(p.Vars) > 0 {
		h.printf("<li><a href=\"#variables\">Variables</a></li>\n")
	}
	for _, f := range p.Funcs {
		h.printf("<li><a href=\"#%s\"><code>%s</code></a></li>\n", f.ID, esc(oneLine(f.Sig)))
	}
	for _, t := range p.Types {
		h.printf("<li><a href=\"#%s\"><code>%s</code></a>", t.ID, esc(indexSig(t)))
		if all := t.indexed(); len(all) > 0 {
			h.printf("\n<ul>\n")
			for _, d := range all {
				h.printf("<li><a href=\"#%s\"><code>%s</code></a></li>\n", d.ID, esc(oneLine(d.Sig)))
			}
			h.printf("</ul>\n")
		}
		h.printf("</li>\n")
	}
	for _, t := range p.Extensions {
		h.printf("<li><a href=\"#%s\"><code>extension %s</code></a></li>\n", t.ID, esc(t.Name))
	}
	if len(p.Other) > 0 {
		h.printf("<li><a href=\"#operators\">Operators</a></li>\n")
	}
	h.printf("</ul>\n")
	if len(p.Files) > 0 {
		h.printf("<p class=\"files\">Package files: %s</p>\n", esc(strings.Join(p.Files, " ")))
	}
	h.printf("</section>\n")

	if len(p.Consts) > 0 {
		h.printf("<section id=\"constants\">\n<h2>Constants</h2>\n")
		for _, d := range p.Consts {
			h.decl(d, "")
		}
		h.printf("</section>\n")
	}
	if len(p.Vars) > 0 {
		h.printf("<section id=\"variables\">\n<h2>Variables</h2>\n")
		for _, d := range p.Vars {
			h.decl(d, "")
		}
		h.printf("</section>\n")
	}
	if len(p.Funcs) > 0 {
		h.printf("<section id=\"functions\">\n<h2>Functions</h2>\n")
		for _, d := range p.Funcs {
			h.decl(d, "func "+d.Name)
		}
		h.printf("</section>\n")
	}
	if len(p.Types) > 0 {
		h.printf("<section id=\"types\">\n<h2>Types</h2>\n")
		for _, t := range p.Types {
			h.typ(t, t.Kind+" "+t.Name)
		}
		h.printf("</section>\n")
	}
	if len(p.Extensions) > 0 {
		h.printf("<section id=\"extensions\">\n<h2>Extensions</h2>\n")
		for _, t := range p.Extensions {
			h.typ(t, "extension "+t.Name)
		}
		h.printf("</section>\n")
	}
	if len(p.Other) > 0 {
		h.printf("<section id=\"operators\">\n<h2>Operators</h2>\n")
		for _, d := range p.Other {
			h.decl(d, "")
		}
		h.printf("</section>\n")
	}
	h.printf("</main>\n</body>\n</html>\n")
	return h.err
}

type htmlWriter struct {
	w     io.Writer
	err   error
	links map[string]string
}

func (h *htmlWriter) printf(format string, args ...any) {
	if h.err == nil {
		_, h.err = fmt.Fprintf(h.w, format, args...)
	}
}

func (h *htmlWriter) decl(d *Decl, title string) {
	id := d.ID
	h.printf("<div class=\"decl\" id=\"%s\">\n", id)
	if title != "" {
		h.printf("<h3><a class=\"self\" href=\"#%s\">%s</a> <span class=\"where\">%s:%d</span></h3>\n",
			id, esc(title), esc(d.File), d.Line)
	}
	h.printf("<pre class=\"sig\">%s</pre>\n", h.code(d.Sig))
	h.comment(d.Doc)
	h.printf("</div>\n")
}

func (h *htmlWriter) typ(t *Type, title string) {
	h.printf("<article class=\"type\">\n")
	h.decl(t.Decl, title)
	if len(t.Conforms) > 0 {
		h.printf("<p class=\"conforms\">Conforms by extension to <code>%s</code></p>\n", h.code(strings.Join(t.Conforms, ", ")))
	}
	for _, g := range t.groups() {
		h.printf("<h4>%s</h4>\n", esc(g.name))
		for _, d := range g.decls {
			h.decl(d, "")
		}
	}
	h.printf("</article>\n")
}

func (h *htmlWriter) comment(text string) {
	for _, b := range blocks(text) {
		switch b.kind {
		case paragraph:
			h.printf("<p>%s</p>\n", h.prose(strings.Join(b.lines, "\n")))
		case heading:
			h.printf("<h4>%s</h4>\n", h.prose(b.lines[0]))
		case list:
			h.printf("<ul>\n")
			for _, it := range b.lines {
				h.printf("<li>%s</li>\n", h.prose(it))
			}
			h.printf("</ul>\n")
		case code:
			h.printf("<pre class=\"example\">%s</pre>\n", h.code(strings.Join(b.lines, "\n")))
		}
	}
}

var inlineCode = regexp.MustCompile("`([^`]+)`")

// prose is comment text as HTML: `code` spans set as code, linked where
// they name one of the package's declarations.
func (h *htmlWriter) prose(s string) string {
	var b strings.Builder
	last := 0
	for _, m := range inlineCode.FindAllStringSubmatchIndex(s, -1) {
		b.WriteString(esc(s[last:m[0]]))
		inner := s[m[2]:m[3]]
		if id, ok := h.links[strings.TrimSuffix(inner, "()")]; ok {
			fmt.Fprintf(&b, "<a href=\"#%s\"><code>%s</code></a>", id, esc(inner))
		} else {
			fmt.Fprintf(&b, "<code>%s</code>", esc(inner))
		}
		last = m[1]
	}
	b.WriteString(esc(s[last:]))
	return b.String()
}

// keywords are the words a signature is highlighted by.
var keywords = map[string]bool{}

func init() {
	for _, k := range strings.Fields(`actor any as associatedtype async await borrowing break case catch class
		consuming continue convenience default defer deinit didSet do dynamic else enum extension
		fallthrough false fileprivate final for func get graph guard if import in indirect infix init
		inout internal is isolated kernel lazy let mutating nil nonisolated nonmutating open operator
		optional override package postfix precedencegroup prefix private protocol public repeat
		required rethrows return self Self set some static struct subscript super switch throw throws
		true try typealias unowned var weak where while willSet macro`) {
		keywords[k] = true
	}
}

// code is Vertex source as highlighted HTML: keywords, types, literals,
// comments and attributes, with the package's own names linked.
func (h *htmlWriter) code(src string) string {
	var b strings.Builder
	span := func(class, text string) {
		fmt.Fprintf(&b, "<span class=\"%s\">%s</span>", class, esc(text))
	}
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			j := strings.IndexByte(src[i:], '\n')
			if j < 0 {
				j = len(src) - i
			}
			span("c", src[i:i+j])
			i += j
		case c == '"':
			j := i + 1
			for j < len(src) && src[j] != '"' && src[j] != '\n' {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			j = min(j+1, len(src))
			span("s", src[i:j])
			i = j
		case c == '@' || c == '#':
			j := i + 1
			for j < len(src) && isIdent(src[j]) {
				j++
			}
			span("a", src[i:j])
			i = j
		case c >= '0' && c <= '9':
			j := i
			for j < len(src) && (isIdent(src[j]) || src[j] == '.') {
				j++
			}
			span("n", src[i:j])
			i = j
		case isIdent(c) || c == '`':
			j := i + 1
			for j < len(src) && (isIdent(src[j]) || (c == '`' && src[j] != '`')) {
				j++
			}
			if c == '`' && j < len(src) {
				j++
			}
			word := src[i:j]
			switch {
			case keywords[word]:
				span("k", word)
			case h.links[word] != "" && word[0] >= 'A' && word[0] <= 'Z':
				fmt.Fprintf(&b, "<a class=\"t\" href=\"#%s\">%s</a>", h.links[word], esc(word))
			case word[0] >= 'A' && word[0] <= 'Z':
				span("t", word)
			default:
				b.WriteString(esc(word))
			}
			i = j
		default:
			b.WriteString(esc(string(c)))
			i++
		}
	}
	return b.String()
}

func isIdent(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c >= 0x80
}

func esc(s string) string { return html.EscapeString(s) }

const css = `
:root {
  --bg: #ffffff; --fg: #1f2328; --muted: #656d76; --line: #d8dee4;
  --code-bg: #f6f8fa; --link: #0969da;
  --k: #cf222e; --t: #8250df; --s: #0a3069; --n: #0550ae; --c: #6e7781; --a: #953800;
}
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    --bg: #0d1117; --fg: #e6edf3; --muted: #8d96a0; --line: #30363d;
    --code-bg: #161b22; --link: #4493f8;
    --k: #ff7b72; --t: #d2a8ff; --s: #a5d6ff; --n: #79c0ff; --c: #8b949e; --a: #ffa657;
  }
}
:root[data-theme="dark"] {
  --bg: #0d1117; --fg: #e6edf3; --muted: #8d96a0; --line: #30363d;
  --code-bg: #161b22; --link: #4493f8;
  --k: #ff7b72; --t: #d2a8ff; --s: #a5d6ff; --n: #79c0ff; --c: #8b949e; --a: #ffa657;
}
* { box-sizing: border-box; }
body { margin: 0; background: var(--bg); color: var(--fg);
  font: 16px/1.55 -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif; }
main { max-width: 960px; margin: 0 auto; padding: 24px 16px 64px; }
h1 { font-size: 2em; margin: 0 0 12px; }
h2 { font-size: 1.5em; border-bottom: 1px solid var(--line); padding-bottom: 6px; margin-top: 40px; }
h3 { font-size: 1.2em; margin: 28px 0 8px; }
h4 { font-size: 1em; color: var(--muted); text-transform: uppercase; letter-spacing: .04em; margin: 22px 0 8px; }
a { color: var(--link); text-decoration: none; }
a:hover { text-decoration: underline; }
a.self { color: inherit; }
.where { font-size: .75em; color: var(--muted); font-weight: normal; margin-left: 8px; }
pre, code { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 14px; }
code { background: var(--code-bg); padding: .1em .3em; border-radius: 4px; }
pre { background: var(--code-bg); border: 1px solid var(--line); border-radius: 6px;
  padding: 12px 14px; overflow-x: auto; margin: 8px 0 12px; }
pre.sig { border-left: 3px solid var(--link); }
pre code, a code, .index code { background: none; padding: 0; }
.index, .index ul { list-style: none; padding-left: 0; }
.index ul { padding-left: 22px; margin: 2px 0 6px; }
.index li { margin: 3px 0; overflow-wrap: anywhere; }
.files, .conforms { color: var(--muted); }
article.type { margin-bottom: 36px; }
.decl + .decl { margin-top: 4px; }
.k { color: var(--k); } .t { color: var(--t); } .s { color: var(--s); }
.n { color: var(--n); } .c { color: var(--c); font-style: italic; } .a { color: var(--a); }
`

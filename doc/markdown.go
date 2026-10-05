package doc

import (
	"fmt"
	"io"
	"strings"
)

// Fence is the info string code blocks are written with.
const Fence = "vertex"

// Markdown writes the package's documentation as GitHub-flavored Markdown:
// the overview, an index, and every declaration's signature as a code
// block with its comment under it.
func Markdown(w io.Writer, p *Package) error {
	m := &mdWriter{w: w}
	m.printf("# package %s\n\n", p.Name)
	if p.ImportPath != "" {
		m.printf("```%s\nimport \"%s\"\n```\n\n", Fence, p.ImportPath)
	}
	m.comment(p.Doc)

	m.printf("## Index\n\n")
	if len(p.Consts) > 0 {
		m.printf("- [Constants](#constants)\n")
	}
	if len(p.Vars) > 0 {
		m.printf("- [Variables](#variables)\n")
	}
	for _, f := range p.Funcs {
		m.printf("- [%s](#%s)\n", mdCode(oneLine(f.Sig)), f.ID)
	}
	for _, t := range p.Types {
		m.printf("- [%s](#%s)\n", mdCode(indexSig(t)), t.ID)
		for _, d := range t.indexed() {
			m.printf("  - [%s](#%s)\n", mdCode(oneLine(d.Sig)), d.ID)
		}
	}
	for _, t := range p.Extensions {
		m.printf("- [%s](#%s)\n", mdCode("extension "+t.Name), t.ID)
	}
	if len(p.Other) > 0 {
		m.printf("- [Operators](#operators)\n")
	}
	m.printf("\n")

	if len(p.Consts) > 0 {
		m.printf("## Constants\n\n")
		for _, d := range p.Consts {
			m.decl(d, "")
		}
	}
	if len(p.Vars) > 0 {
		m.printf("## Variables\n\n")
		for _, d := range p.Vars {
			m.decl(d, "")
		}
	}
	if len(p.Funcs) > 0 {
		m.printf("## Functions\n\n")
		for _, d := range p.Funcs {
			m.decl(d, "### func "+d.Name)
		}
	}
	if len(p.Types) > 0 {
		m.printf("## Types\n\n")
		for _, t := range p.Types {
			m.typ(t, "### "+t.Kind+" "+t.Name)
		}
	}
	if len(p.Extensions) > 0 {
		m.printf("## Extensions\n\n")
		for _, t := range p.Extensions {
			m.typ(t, "### extension "+t.Name)
		}
	}
	if len(p.Other) > 0 {
		m.printf("## Operators\n\n")
		for _, d := range p.Other {
			m.decl(d, "")
		}
	}
	if len(p.Files) > 0 {
		m.printf("## Files\n\n")
		for _, f := range p.Files {
			m.printf("- %s\n", f)
		}
	}
	return m.err
}

type mdWriter struct {
	w   io.Writer
	err error
}

func (m *mdWriter) printf(format string, args ...any) {
	if m.err == nil {
		_, m.err = fmt.Fprintf(m.w, format, args...)
	}
}

func (m *mdWriter) decl(d *Decl, title string) {
	id := d.ID
	if title != "" {
		m.printf("%s <a id=\"%s\"></a>\n\n", title, id)
	} else {
		m.printf("<a id=\"%s\"></a>\n\n", id)
	}
	m.printf("```%s\n%s\n```\n\n", Fence, d.Sig)
	m.comment(d.Doc)
}

func (m *mdWriter) typ(t *Type, title string) {
	m.decl(t.Decl, title)
	if len(t.Conforms) > 0 {
		m.printf("Conforms by extension to: %s\n\n", mdCode(strings.Join(t.Conforms, ", ")))
	}
	for _, group := range t.groups() {
		m.printf("#### %s\n\n", group.name)
		for _, d := range group.decls {
			m.decl(d, "")
		}
	}
}

// comment writes a doc comment as Markdown: its prose as it is, which is
// Markdown already by habit, and its indented code fenced.
func (m *mdWriter) comment(text string) {
	for _, b := range blocks(text) {
		switch b.kind {
		case paragraph:
			m.printf("%s\n\n", strings.Join(b.lines, "\n"))
		case heading:
			m.printf("**%s**\n\n", b.lines[0])
		case list:
			for _, it := range b.lines {
				m.printf("- %s\n", it)
			}
			m.printf("\n")
		case code:
			m.printf("```%s\n%s\n```\n\n", Fence, strings.Join(b.lines, "\n"))
		}
	}
}

// mdCode is s as inline code, with enough backticks to hold any in it.
func mdCode(s string) string {
	fence := "`"
	for strings.Contains(s, fence) {
		fence += "`"
	}
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		return fence + " " + s + " " + fence
	}
	return fence + s + fence
}

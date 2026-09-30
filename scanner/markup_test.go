package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertex-language/vsc/token"
)

// spell renders a token stream compactly: markup kinds by their text,
// everything else by kind.
func spell(f *token.File, toks []token.Token) string {
	var b strings.Builder
	for _, tk := range toks {
		if tk.Kind == token.EOF {
			break
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		text := string(f.Slice(tk.Pos, tk.End))
		switch tk.Kind {
		case token.MARKUP_NAME, token.MARKUP_STRING, token.MARKUP_TEXT, token.IDENT, token.INT_LIT:
			b.WriteString(tk.Kind.String() + "(" + text + ")")
		default:
			b.WriteString(text)
		}
	}
	return b.String()
}

func scanMarkup(t *testing.T, src string) (string, []token.Diagnostic) {
	t.Helper()
	f := token.NewFile("m.vsx", []byte(src))
	toks, diags := Scan(f, ScanMarkup)
	return spell(f, toks), diags
}

func TestMarkupTokens(t *testing.T) {
	cases := []struct{ src, want string }{
		{`return <b class="x">Hi {n}</b>`,
			`return < MARKUP_NAME(b) MARKUP_NAME(class) = MARKUP_STRING("x") > MARKUP_TEXT(Hi ) { IDENT(n) } </ MARKUP_NAME(b) >`},
		{`let a = <input value={$draft} autofocus />`,
			`let IDENT(a) = < MARKUP_NAME(input) MARKUP_NAME(value) = { IDENT($draft) } MARKUP_NAME(autofocus) />`},
		{`<app.Window title='T'><Counter start={5} /></app.Window>`,
			`< MARKUP_NAME(app.Window) MARKUP_NAME(title) = MARKUP_STRING('T') > < MARKUP_NAME(Counter) MARKUP_NAME(start) = { INT_LIT(5) } /> </ MARKUP_NAME(app.Window) >`},
		{`(<><i/></>)`,
			`( < > < MARKUP_NAME(i) /> </ > )`},
		{`<li class:done={d} style:--kit-accent={c} data-p="m"/>`,
			`< MARKUP_NAME(li) MARKUP_NAME(class:done) = { IDENT(d) } MARKUP_NAME(style:--kit-accent) = { IDENT(c) } MARKUP_NAME(data-p) = MARKUP_STRING("m") />`},
		// Code in markup opens markup again, and braces written in it
		// are its own.
		{`<ul>{if a { <li/> } else { f({ 1 }) }}</ul>`,
			`< MARKUP_NAME(ul) > { if IDENT(a) { < MARKUP_NAME(li) /> } else { IDENT(f) ( { INT_LIT(1) } ) } } </ MARKUP_NAME(ul) >`},
		// A string in code holds a `}` without closing anything.
		{`<p>{"}"}</p>`,
			`< MARKUP_NAME(p) > { " } " } </ MARKUP_NAME(p) >`},
		{`<a onClick={e in go(e)}>x</a>`,
			`< MARKUP_NAME(a) MARKUP_NAME(onClick) = { IDENT(e) in IDENT(go) ( IDENT(e) ) } > MARKUP_TEXT(x) </ MARKUP_NAME(a) >`},
		{`<p>{/* note */}</p>`,
			`< MARKUP_NAME(p) > { } </ MARKUP_NAME(p) >`},
		{`<p>{...attrs}</p>`,
			`< MARKUP_NAME(p) > { ... IDENT(attrs) } </ MARKUP_NAME(p) >`},
	}
	for _, c := range cases {
		got, diags := scanMarkup(t, c.src)
		if len(diags) != 0 {
			t.Errorf("%s: unexpected diagnostics: %v", c.src, diags)
		}
		if got != c.want {
			t.Errorf("%s\n got: %s\nwant: %s", c.src, got, c.want)
		}
	}
}

// `<` bound on the left, or not bound on the right by a name or `>`, is
// the operator it always was.
func TestMarkupLeavesOperators(t *testing.T) {
	for _, src := range []string{
		`a < b`, `a<b`, `Array<int>()`, `func f<T>(x: T) {}`, `xs.sorted(by: <)`,
		`let y = x < 3 ? a : b`, `if a <= b {}`, `x <<= 2`, `let s = "<b>"`,
		// A declared name's generic parameters, written apart from it.
		`func |> <T, U>(a: T, f: (T) -> U) -> U { f(a) }`, `func f <T>(x: T) {}`,
		`struct S <T> {}`, `init <T>(x: T) {}`, `actor A <T> {}`,
	} {
		f := token.NewFile("m.vsx", []byte(src))
		toks, diags := Scan(f, ScanMarkup)
		if len(diags) != 0 {
			t.Errorf("%s: unexpected diagnostics: %v", src, diags)
		}
		for _, tk := range toks {
			if tk.Kind.IsMarkup() {
				t.Errorf("%s: scanned markup: %s", src, spell(f, toks))
				break
			}
		}
	}
}

// A .vs file never scans markup, even where a .vsx file would.
func TestNoMarkupWithoutMode(t *testing.T) {
	f := token.NewFile("m.vs", []byte(`return <b>x</b>`))
	toks, _ := Scan(f, 0)
	for _, tk := range toks {
		if tk.Kind.IsMarkup() {
			t.Fatalf("scanned markup in a .vs file: %s", spell(f, toks))
		}
	}
}

func TestMarkupDiagnostics(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`<p>a > b</p>`, "'>' cannot appear in markup text"},
		{`<p>a } b</p>`, "'}' cannot appear in markup text"},
		{`<p>a < b</p>`, "'<' cannot appear in markup text"},
		{`<p>open`, "element has no closing tag"},
		{`<p class="x`, "unterminated attribute value"},
		{`<p a={x`, "unterminated '{' in markup"},
		{`<p %>`, "this character cannot appear in a tag"},
	} {
		_, diags := scanMarkup(t, c.src)
		found := false
		for _, d := range diags {
			if strings.Contains(d.Message, c.want) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: want a diagnostic %q, got %v", c.src, c.want, diags)
		}
	}
}

// Renaming a .vs file to .vsx changes nothing, unless it applies a prefix
// operator spelled with `<`: every program in the ladder scans to the
// same tokens with markup on as with it off.
func TestMarkupModeKeepsVertex(t *testing.T) {
	files, _ := filepath.Glob("../tests/*.swift")
	more, _ := filepath.Glob("../parser/testdata/syntax/*")
	files = append(files, more...)
	if len(files) == 0 {
		t.Skip("no corpus")
	}
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		plain := token.NewFile(path, src)
		a, _ := Scan(plain, 0)
		marked := token.NewFile(path, src)
		b, _ := Scan(marked, ScanMarkup)
		if len(a) != len(b) {
			t.Errorf("%s: %d tokens without markup, %d with", path, len(a), len(b))
			continue
		}
		for i := range a {
			if a[i].Kind != b[i].Kind || a[i].Pos != b[i].Pos || a[i].End != b[i].End {
				t.Errorf("%s: token %d differs: %v vs %v", path, i, a[i].Kind, b[i].Kind)
				break
			}
		}
	}
}

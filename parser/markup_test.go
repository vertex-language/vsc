package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
)

// The RFC's own examples parse, with markup where it is written and
// nowhere else.
func TestMarkupFiles(t *testing.T) {
	files, _ := filepath.Glob("../tests/vsx/syntax/*.vsx")
	if len(files) == 0 {
		t.Fatal("no ../tests/vsx/syntax/*.vsx")
	}
	for _, name := range files {
		t.Run(filepath.Base(name), func(t *testing.T) {
			src, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			f := token.NewFile(name, src)
			file, diags := ParseFile(f, 0)
			for _, d := range diags {
				t.Errorf("%s", d.Print(f))
			}
			elements := 0
			ast.Inspect(file, func(n ast.Node) bool {
				if _, ok := n.(*ast.MarkupElement); ok {
					elements++
				}
				return true
			})
			if elements == 0 {
				t.Errorf("no markup found")
			}
		})
	}
}

func parseVsx(t *testing.T, src string) (*ast.File, *token.File, []token.Diagnostic) {
	t.Helper()
	f := token.NewFile("t.vsx", []byte(src))
	file, diags := ParseFile(f, 0)
	return file, f, diags
}

// firstMarkup is the first element in the file.
func firstMarkup(file *ast.File) *ast.MarkupElement {
	var el *ast.MarkupElement
	ast.Inspect(file, func(n ast.Node) bool {
		if e, ok := n.(*ast.MarkupElement); ok && el == nil {
			el = e
		}
		return el == nil
	})
	return el
}

func TestMarkupTree(t *testing.T) {
	file, f, diags := parseVsx(t, `let x = <a href="/" onClick={e in go(e)} hidden {...rest}>Hi {name}<b/></a>`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	el := firstMarkup(file)
	if el == nil || el.Name.Text(f) != "a" || el.CloseName.Text(f) != "a" || el.SelfClosing {
		t.Fatalf("bad element: %+v", el)
	}
	if len(el.Attrs) != 4 {
		t.Fatalf("want 4 attributes, got %d", len(el.Attrs))
	}
	href := el.Attrs[0].(*ast.MarkupAttribute)
	if href.Name.Text(f) != "href" || href.Value.(*ast.MarkupString).Value(f) != "/" {
		t.Errorf("href: %+v", href)
	}
	click := el.Attrs[1].(*ast.MarkupAttribute).Value.(*ast.MarkupCode)
	if click.Body.Sig == nil || len(click.Body.Stmts) != 1 {
		t.Errorf("onClick body should be a closure with a parameter and one statement")
	}
	if el.Attrs[2].(*ast.MarkupAttribute).Value != nil {
		t.Errorf("hidden should have no value")
	}
	if _, ok := el.Attrs[3].(*ast.MarkupSpread); !ok {
		t.Errorf("want a spread, got %T", el.Attrs[3])
	}
	if len(el.Children) != 3 {
		t.Fatalf("want 3 children, got %d", len(el.Children))
	}
	if _, ok := el.Children[0].(*ast.MarkupText); !ok {
		t.Errorf("child 0: %T", el.Children[0])
	}
	if _, ok := el.Children[1].(*ast.MarkupCode); !ok {
		t.Errorf("child 1: %T", el.Children[1])
	}
	if b, ok := el.Children[2].(*ast.MarkupElement); !ok || !b.SelfClosing {
		t.Errorf("child 2: %T", el.Children[2])
	}
}

func TestMarkupFragment(t *testing.T) {
	file, _, diags := parseVsx(t, `let x = <><i/>{a}</>`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	el := firstMarkup(file)
	if el.Name != nil || el.CloseName != nil || len(el.Children) != 2 {
		t.Errorf("bad fragment: %+v", el)
	}
}

// A `{` after an element is never its trailing closure.
func TestMarkupTakesNoTrailingClosure(t *testing.T) {
	file, _, diags := parseVsx(t, "func f() {\n    let a = <b/>\n    { print(1) }()\n}")
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	var decl *ast.VarDecl
	ast.Inspect(file, func(n ast.Node) bool {
		if d, ok := n.(*ast.VarDecl); ok {
			decl = d
		}
		return true
	})
	if _, ok := decl.Bindings[0].Value.(*ast.MarkupElement); !ok {
		t.Errorf("value is %T, want the element alone", decl.Bindings[0].Value)
	}
}

func TestMarkupErrors(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`let x = <a></b>`, "expected </a> to close <a>"},
		{`let x = <a></>`, "expected </a>"},
		{`let x = <></a>`, "a fragment closes with </>"},
		{`let x = < />`, ""}, // not markup: `<` is not bound on the right
		{`let x = <a b= />`, "expected a quoted value or '{' after '='"},
		{`let x = <a b="1" {c} />`, "expected an attribute, '>' or '/>'"},
	} {
		_, f, diags := parseVsx(t, c.src)
		if c.want == "" {
			continue
		}
		var msgs []string
		for _, d := range diags {
			msgs = append(msgs, d.Print(f))
		}
		if !strings.Contains(strings.Join(msgs, "\n"), c.want) {
			t.Errorf("%s: want %q in\n%s", c.src, c.want, strings.Join(msgs, "\n"))
		}
	}
}

// A .vs file has no markup: the same text is an error there, not an element.
func TestNoMarkupInVs(t *testing.T) {
	f := token.NewFile("t.vs", []byte(`let x = <b/>`))
	file, _ := ParseFile(f, 0)
	if firstMarkup(file) != nil {
		t.Errorf("a .vs file parsed markup")
	}
}

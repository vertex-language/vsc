package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
)

func TestFiles(t *testing.T) {
	var files []string
	if env := os.Getenv("VSC_FILES"); env != "" {
		files = strings.Split(env, ",")
	} else {
		// The grammar corpus, and the ladder: every rung is a program
		// swiftc builds, so every rung has to parse before it can run.
		syntax, _ := filepath.Glob("testdata/syntax/*.swift")
		ladder, _ := filepath.Glob("../tests/*.swift")
		if len(syntax) == 0 || len(ladder) == 0 {
			t.Fatal("no test files found in testdata/syntax/*.swift or ../tests/*.swift")
		}
		files = append(syntax, ladder...)
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
			if file == nil {
				t.Fatalf("expected non-nil AST for %s", name)
			}

			// Validate that every node in the AST is traversable by Inspect
			var nodeCount int
			ast.Inspect(file, func(n ast.Node) bool {
				if n != nil {
					nodeCount++
				}
				return true
			})
			if nodeCount == 0 {
				t.Errorf("expected non-zero AST node count for %s", name)
			}

			// Validate that Fdump works cleanly without errors
			var sb strings.Builder
			if err := ast.Fdump(&sb, f, file); err != nil {
				t.Errorf("Fdump failed on %s: %v", name, err)
			}
			if os.Getenv("VSC_DUMP") != "" {
				t.Log("\n" + sb.String())
			}
		})
	}
}

func TestParseComments(t *testing.T) {
	src := "// top comment\nlet x = 1\n/* block */"
	f := token.NewFile("comment.vs", []byte(src))
	file, diags := ParseFile(f, ParseComments)
	if len(diags) != 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if len(file.Comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(file.Comments))
	}
	defer file.Release()
}

func TestParseSkipBodies(t *testing.T) {
	src := `func calculate(x: Int) -> Int { let a = 1; let b = 2; return a + b }`
	f := token.NewFile("skip.vs", []byte(src))
	file, diags := ParseFile(f, SkipBodies)
	if len(diags) != 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if len(file.Stmts) != 1 {
		t.Errorf("expected 1 stmt, got %d", len(file.Stmts))
	}
	defer file.Release()
}

func TestParseErrorRecovery(t *testing.T) {
	src := `let = 10; func (x: Int) {}; if { let y = 2 }`
	f := token.NewFile("err.vs", []byte(src))
	file, diags := ParseFile(f, Tolerant)
	if len(diags) == 0 {
		t.Errorf("expected parse errors")
	}
	if file == nil {
		t.Errorf("expected non-nil AST even with errors")
	}
	defer file.Release()
}

// TestQualifiedEnumCasePattern: `Module.Type.case(...)` in a pattern is an
// enum case pattern whose type is qualified, not an expression to compare
// with. Swift's grammar gives an enum case pattern a TypeIdentifier, which
// is dotted; only its last member is the case. Read as an expression, a
// catch clause naming a case of another module's error type could not be
// lowered.
func TestQualifiedEnumCasePattern(t *testing.T) {
	src := "func f() { do { try g() } catch tcp.TcpError.timedOut(let what) { } catch E.x { } }"
	f := token.NewFile("a.swift", []byte(src))
	file, diags := ParseFile(f, 0)
	for _, d := range diags {
		t.Fatalf("%s", d.Print(f))
	}
	var pats []ast.Pattern
	ast.Inspect(file, func(n ast.Node) bool {
		if cl, ok := n.(*ast.CatchClause); ok {
			for _, it := range cl.Items {
				pats = append(pats, it.Pat)
			}
		}
		return true
	})
	if len(pats) != 2 {
		t.Fatalf("%d catch patterns, want 2", len(pats))
	}
	ec, ok := pats[0].(*ast.EnumCasePattern)
	if !ok {
		t.Fatalf("the qualified pattern is %T, want an enum case pattern", pats[0])
	}
	mt, ok := ec.Type.(*ast.MemberType)
	if !ok {
		t.Fatalf("its type is %T, want a qualified type", ec.Type)
	}
	if mt.Name.Text(f) != "TcpError" || ec.Name.Text(f) != "timedOut" || ec.Args == nil {
		t.Errorf("read as %s.%s, want TcpError.timedOut with a binding", mt.Name.Text(f), ec.Name.Text(f))
	}
	if _, ok := pats[1].(*ast.EnumCasePattern); !ok {
		t.Errorf("the unqualified pattern is %T, want an enum case pattern", pats[1])
	}
}

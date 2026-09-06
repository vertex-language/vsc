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
		var err error
		files, err = filepath.Glob("../tests/syntax/*.swift")
		if err != nil || len(files) == 0 {
			files, err = filepath.Glob("tests/syntax/*.swift")
			if err != nil || len(files) == 0 {
				t.Fatal("no test files found in tests/syntax/*.swift or ../tests/syntax/*.swift")
			}
		}
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

package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
)

// cmdAST parses one file and dumps the syntax tree.
func cmdAST(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ast", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	names := fs.Args()
	if len(names) > 1 {
		fmt.Fprintln(stderr, "vsc: ast takes one file")
		return exitUsage
	}
	src, err := source(first(names))
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}

	f := token.NewFile(src.Name, src.Text)
	file, diags := parser.ParseFile(f, 0)
	if err := ast.Fdump(stdout, f, file); err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}
	code := exitOK
	for _, d := range diags {
		fmt.Fprintln(stderr, d.Print(f))
		if d.Severity == token.Error {
			code = exitDiags
		}
	}
	return code
}

// first returns the first name or "-" if empty.
func first(names []string) string {
	if len(names) == 0 {
		return "-"
	}
	return names[0]
}

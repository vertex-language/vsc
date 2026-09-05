package cli

import (
	"flag"
	"fmt"
	"io"
	"strconv"

	"github.com/vertex-language/vsc/scanner"
	"github.com/vertex-language/vsc/token"
)

// cmdTokens scans one file and prints the token stream.
//
// Position, kind, and the bytes the token was written as — which is
// the whole of what a token is here, since the scanner interprets
// nothing and a literal keeps its spelling. It is the first thing to
// reach for when a parse went wrong in a way the tree does not
// explain.
func cmdTokens(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tokens", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	names := fs.Args()
	if len(names) > 1 {
		fmt.Fprintln(stderr, "vsc: tokens takes one file")
		return exitUsage
	}
	src, err := source(first(names))
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}

	f := token.NewFile(src.Name, src.Text)
	toks, diags := scanner.Scan(f, 0)
	text := f.Text()
	for _, t := range toks {
		p := f.Position(t.Pos)
		lit := ""
		if lo, hi := f.Offset(t.Pos), f.Offset(t.End); lo >= 0 && hi <= len(text) && lo < hi {
			lit = strconv.Quote(string(text[lo:hi]))
		}
		fmt.Fprintf(stdout, "%d:%d\t%s\t%s\n", p.Line, p.Column, t.Kind, lit)
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

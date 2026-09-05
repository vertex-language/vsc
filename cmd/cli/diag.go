package cli

import (
	"bytes"
	"fmt"
	"io"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/token"
)

// One renderer, for one kind of diagnostic.
//
// Every diagnostic the library returns already knows where it is —
// Swift has no preprocessor, so the bytes the scanner read are the
// bytes that were typed and a span means the same thing to every
// phase. Nothing is left to work out here; what is left is
// presentation: the message, the source line, and the caret under it.

// printDiags renders each diagnostic and reports whether any of them
// was an error.
func printDiags(w io.Writer, diags []vsc.Diagnostic) bool {
	for _, d := range diags {
		fmt.Fprintln(w, d.String())
		if d.File != nil {
			printSnippet(w, d.File, d.Pos, d.End)
		}
	}
	return vsc.Errors(diags)
}

// printSnippet draws the line a span is on and underlines the span.
//
// A diagnostic with no position — one a pass reported about a module
// rather than about a line — draws nothing, which is why the caller
// checks the file and this checks the position.
func printSnippet(w io.Writer, f *token.File, pos, end token.Pos) {
	if !pos.IsValid() {
		return
	}
	src := f.Text()
	p := f.Position(pos)
	if p.Offset < 0 || p.Offset > len(src) {
		return
	}

	// The line the span starts on. Column is one-based and in bytes,
	// so the start of the line is the offset less the column.
	lo := p.Offset - (p.Column - 1)
	hi := p.Offset
	for hi < len(src) && src[hi] != '\n' && src[hi] != '\r' {
		hi++
	}
	if lo < 0 || lo > hi {
		return
	}
	line := src[lo:hi]

	// The underline is the span, clamped to this line: a span that
	// runs to the end of the file underlines to the end of its line
	// rather than off it.
	width := 1
	if end.IsValid() && end > pos {
		width = f.Offset(end) - p.Offset
	}
	if p.Column-1+width > len(line) {
		width = len(line) - (p.Column - 1)
	}
	if width < 1 {
		width = 1
	}

	// Tabs stay tabs in the pad so the caret lands under the column
	// the terminal actually put the character in.
	pad := make([]byte, p.Column-1)
	for i := range pad {
		if line[i] == '\t' {
			pad[i] = '\t'
		} else {
			pad[i] = ' '
		}
	}
	fmt.Fprintf(w, "    %s\n    %s%s\n", line, pad, bytes.Repeat([]byte("^"), width))
}

package token

import (
	"strings"
	"testing"
)

// Printing a diagnostic must not be able to stop the compilation.
//
// A position is an offset into one file and carries no mark of which,
// so a diagnostic about an imported declaration handed to the
// program's file is asking for a line that is not there. That used to
// panic inside File.Offset: the compiler died while trying to explain
// itself, and the message it was going to print never appeared.

func TestPrintPlacesADiagnosticOrSaysNothing(t *testing.T) {
	program := NewFile("program.swift", []byte("func main() -> Int32 { return 0 }\n"))
	other := NewFile("Lib.vertexinterface", []byte(strings.Repeat("public func f() -> Int32\n", 60)))

	t.Run("a position in the file it is given", func(t *testing.T) {
		d := Diagnostic{Pos: program.Pos(5), End: program.Pos(9), Severity: Error, Message: "no"}
		if got, want := d.Print(program), "program.swift:1:6: error: no"; got != want {
			t.Errorf("got  %s\nwant %s", got, want)
		}
	})

	t.Run("a position past the end of it", func(t *testing.T) {
		d := Diagnostic{Pos: other.Pos(900), End: other.Pos(900), Severity: Error, Message: "no"}
		got := d.Print(program)
		if strings.Contains(got, ":1:") || strings.Contains(got, ":9") {
			t.Errorf("got %q, which puts the mistake on a line of the wrong file", got)
		}
		if !strings.Contains(got, "program.swift") || !strings.Contains(got, "no") {
			t.Errorf("got %q, want the file it was given and the message", got)
		}
	})

	t.Run("its own file wins over the one it is given", func(t *testing.T) {
		d := Diagnostic{Pos: other.Pos(900), End: other.Pos(900), Severity: Error,
			Message: "no", File: other}
		got := d.Print(program)
		if !strings.HasPrefix(got, "Lib.vertexinterface:37:1:") {
			t.Errorf("got %q, want it placed in the file it says it is in", got)
		}
	})

	t.Run("no position at all names no file", func(t *testing.T) {
		d := Diagnostic{Severity: Error, Message: "about the module"}
		if got, want := d.Print(program), "error: about the module"; got != want {
			t.Errorf("got  %s\nwant %s", got, want)
		}
	})

	t.Run("no file at all", func(t *testing.T) {
		d := Diagnostic{Pos: program.Pos(5), Severity: Error, Message: "no"}
		if got, want := d.Print(nil), "error: no"; got != want {
			t.Errorf("got  %s\nwant %s", got, want)
		}
	})
}

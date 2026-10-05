package token

import (
	"fmt"
	"sort"
)

// Severity classifies a diagnostic.
type Severity uint8

const (
	Note Severity = iota
	Warn
	Error
)

func (s Severity) String() string {
	switch s {
	case Note:
		return "note"
	case Warn:
		return "warning"
	case Error:
		return "error"
	}
	return "severity(" + itoa(int(s)) + ")"
}

// Diagnostic is one report with a non-empty span in some File's
// position space.
//
// File is the one that span belongs to, where whoever raised the
// diagnostic knew. It matters because a position is an offset into
// one file and nothing else: two files' positions look alike, so a
// span rendered against the wrong file names a line that has nothing
// to do with the mistake -- and, once the span is past that file's
// end, cannot name one at all.
//
// It is optional. A diagnostic that does not carry its file is
// rendered against whichever one Print is given, which is right
// wherever there is only one file in play.
type Diagnostic struct {
	Pos      Pos
	End      Pos
	Severity Severity
	Message  string
	File     *File
}

// Print renders the diagnostic: name:line:col: severity: message.
//
// The diagnostic's own file wins over the one passed in, because it
// is the one that knows. A span that belongs to neither is rendered
// without a line rather than against a line it is not on: a compiler
// may fail to place a mistake, but a printer that stops the process
// while trying is worse than a message with no line number on it.
func (d Diagnostic) Print(f *File) string {
	if d.File != nil {
		f = d.File
	}
	// A diagnostic about no particular line -- one raised about a
	// module, or about a declaration in a file this does not have --
	// names no file either. Naming one it was merely handed would put
	// the reader in the wrong place to look.
	if f == nil || !d.Pos.IsValid() {
		return fmt.Sprintf("%s: %s", d.Severity, d.Message)
	}
	if !f.Contains(d.Pos) {
		return fmt.Sprintf("%s: %s: %s", f.Name(), d.Severity, d.Message)
	}
	p := f.Position(d.Pos)
	return fmt.Sprintf("%s:%d:%d: %s: %s", p.Filename, p.Line, p.Column, d.Severity, d.Message)
}

// SortDiagnostics orders by position, then extent, then message,
// stably — so merged scanner and parser slices interleave
// deterministically.
func SortDiagnostics(ds []Diagnostic) {
	sort.SliceStable(ds, func(i, j int) bool {
		a, b := ds[i], ds[j]
		if a.Pos != b.Pos {
			return a.Pos < b.Pos
		}
		if a.End != b.End {
			return a.End < b.End
		}
		return a.Message < b.Message
	})
}

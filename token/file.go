package token

import (
	"fmt"
	"sort"
)

// Position is a resolved location in a File. Line and Column are
// 1-based; Offset is a byte offset, and Column counts bytes, not
// scalars — a diagnostic's underline is drawn from the span, and
// every editor that reads a column expects the byte one.
type Position struct {
	Filename string
	Offset   int
	Line     int
	Column   int
}

func (p Position) String() string {
	return fmt.Sprintf("%s:%d:%d", p.Filename, p.Line, p.Column)
}

// File owns one source file's position space.
//
// It has no translation phase to run. Swift has no trigraphs, no line
// splices, and no preprocessor — #if is a statement — so the bytes
// the scanner reads are the bytes the user typed, and a span means
// the same thing to the scanner, the parser, and the reader of a
// diagnostic. That is why there is no Raw here, and no Diagnostics:
// nothing is reported before scanning.
type File struct {
	name       string
	src        []byte
	lineStarts []int32
}

// NewFile returns the position space for src.
func NewFile(name string, src []byte) *File {
	f := &File{name: name, src: src}
	f.scanLines()
	return f
}

// scanLines records line starts. \n, \r\n, and lone \r all terminate
// a line — the grammar's LineTerminator.
func (f *File) scanLines() {
	starts := []int32{0}
	src := f.src
	for i := 0; i < len(src); i++ {
		switch src[i] {
		case '\n':
			starts = append(starts, int32(i+1))
		case '\r':
			if i+1 < len(src) && src[i+1] == '\n' {
				i++
			}
			starts = append(starts, int32(i+1))
		}
	}
	f.lineStarts = starts
}

// Name returns the file name given to NewFile.
func (f *File) Name() string { return f.name }

// Text returns the source bytes — what the scanner reads.
func (f *File) Text() []byte { return f.src }

// Size is the length of the source. Pos(Size()) is the valid
// one-past-the-end position (the scanner's EOF token).
func (f *File) Size() int { return len(f.src) }

// LineCount is the number of lines, counting a final unterminated one.
func (f *File) LineCount() int { return len(f.lineStarts) }

// Pos converts an offset in [0, Size()] to a Pos.
func (f *File) Pos(offset int) Pos {
	if offset < 0 || offset > len(f.src) {
		panic(fmt.Sprintf("token: Pos offset %d out of range [0, %d]", offset, len(f.src)))
	}
	return Pos(offset + 1)
}

// Offset converts a Pos back to a byte offset.
func (f *File) Offset(p Pos) int {
	if !p.IsValid() || int(p) > len(f.src)+1 {
		panic(fmt.Sprintf("token: invalid Pos %d for file %q", p, f.name))
	}
	return int(p) - 1
}

// Slice returns the bytes of a span — the spelling of whatever the
// span covers. Feed this to decoders.
func (f *File) Slice(pos, end Pos) []byte {
	return f.src[f.Offset(pos):f.Offset(end)]
}

// Position resolves a Pos to offset, line, and column.
func (f *File) Position(p Pos) Position {
	off := f.Offset(p)
	i := sort.Search(len(f.lineStarts), func(i int) bool {
		return int(f.lineStarts[i]) > off
	}) - 1
	return Position{
		Filename: f.name,
		Offset:   off,
		Line:     i + 1,
		Column:   off - int(f.lineStarts[i]) + 1,
	}
}

// Line returns the 1-based line a Pos falls on. It is Position's line
// without the rest of the work — the scanner asks this often, for the
// grammar's line-sensitive rules.
func (f *File) Line(p Pos) int {
	off := f.Offset(p)
	return sort.Search(len(f.lineStarts), func(i int) bool {
		return int(f.lineStarts[i]) > off
	})
}

// LineText returns line n (1-based) without its terminator — for a
// diagnostic renderer that underlines the offending source.
func (f *File) LineText(n int) []byte {
	if n < 1 || n > len(f.lineStarts) {
		return nil
	}
	lo := int(f.lineStarts[n-1])
	hi := len(f.src)
	if n < len(f.lineStarts) {
		hi = int(f.lineStarts[n])
	}
	for hi > lo && (f.src[hi-1] == '\n' || f.src[hi-1] == '\r') {
		hi--
	}
	return f.src[lo:hi]
}

// Between returns the trivia — whitespace and comments — between two
// tokens, for formatters.
func (f *File) Between(prev, next Token) []byte {
	lo, hi := f.Offset(prev.End), f.Offset(next.Pos)
	if lo > hi {
		lo = hi
	}
	return f.src[lo:hi]
}

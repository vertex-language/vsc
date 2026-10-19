package token

import (
	"fmt"
	"sort"
	"unsafe"
)

// Position is a resolved location in a File. Line, Column, and Offset count 1-based bytes.
type Position struct {
	Filename string
	Offset   int
	Line     int
	Column   int
}

func (p Position) String() string {
	return fmt.Sprintf("%s:%d:%d", p.Filename, p.Line, p.Column)
}

// File represents a source file and its position coordinate space.
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

// scanLines records line start byte offsets for \n, \r\n, and lone \r.
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

// Contains reports whether p falls within this file's position range.
func (f *File) Contains(p Pos) bool {
	return f != nil && p.IsValid() && int(p) <= len(f.src)+1
}

// Offset converts a Pos back to a byte offset.
func (f *File) Offset(p Pos) int {
	if !p.IsValid() || int(p) > len(f.src)+1 {
		panic(fmt.Sprintf("token: invalid Pos %d for file %q", p, f.name))
	}
	return int(p) - 1
}

// Slice returns the source bytes of a span.
func (f *File) Slice(pos, end Pos) []byte {
	return f.src[f.Offset(pos):f.Offset(end)]
}

// String returns the span as a string, zero-copy referencing the immutable file source.
func (f *File) String(pos, end Pos) string {
	b := f.Slice(pos, end)
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(&b[0], len(b))
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

// Line returns the 1-based line number for Pos via binary search.
func (f *File) Line(p Pos) int {
	off := f.Offset(p)
	return sort.Search(len(f.lineStarts), func(i int) bool {
		return int(f.lineStarts[i]) > off
	})
}

// LineText returns line n (1-based) without its line terminator.
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

// Between returns whitespace and comments between two tokens.
func (f *File) Between(prev, next Token) []byte {
	lo, hi := f.Offset(prev.End), f.Offset(next.Pos)
	if lo > hi {
		lo = hi
	}
	return f.src[lo:hi]
}

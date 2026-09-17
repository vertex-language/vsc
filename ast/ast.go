package ast

import "github.com/vertex-language/vsc/token"

// Node is the interface all tree nodes implement.
type Node interface {
	Pos() token.Pos // first byte
	End() token.Pos // one past the last byte
}

// Span is the stored extent every node embeds.
type Span struct {
	Lo token.Pos // inclusive
	Hi token.Pos // exclusive
}

func (s Span) Pos() token.Pos { return s.Lo }
func (s Span) End() token.Pos { return s.Hi }

// The five hierarchies. Marker methods are unexported, so the
// hierarchies are closed.

type Expr interface {
	Node
	exprNode()
}

type Stmt interface {
	Node
	stmtNode()
}

type Decl interface {
	Node
	declNode()
}

type Type interface {
	Node
	typeNode()
}

type Pattern interface {
	Node
	patternNode()
}

// Ident represents an identifier AST token span.
type Ident struct {
	Span
	// Escaped indicates whether the identifier was written with backticks.
	Escaped bool
}

// Name returns the identifier's spelling, backticks included.
func (id *Ident) Name(f *token.File) string {
	if id == nil {
		return ""
	}
	return string(f.Slice(id.Lo, id.Hi))
}

// Text returns the identifier's unescaped text name.
func (id *Ident) Text(f *token.File) string {
	s := id.Name(f)
	if id != nil && id.Escaped && len(s) >= 2 {
		return s[1 : len(s)-1]
	}
	return s
}

// Releaser releases the parser arena backing AST storage.
type Releaser interface {
	Release()
}

// File represents a parsed source file AST.
type File struct {
	Span
	Unit     *token.File   // the position space every span resolves through
	Stmts    []Stmt        // the top level, in written order
	Comments []token.Token // retained under parser.ParseComments

	rel Releaser
}

// SetReleaser attaches the tree's backing storage.
func (f *File) SetReleaser(r Releaser) { f.rel = r }

// Release releases the tree's backing storage arena.
func (f *File) Release() {
	if f.rel != nil {
		r := f.rel
		f.rel = nil
		r.Release()
	}
}

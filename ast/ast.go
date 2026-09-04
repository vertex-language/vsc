// Package ast defines the syntax tree vsc's parser builds.
//
// Five hierarchies — Expr, Stmt, Decl, Type, and Pattern — mirroring
// the five families of docs/swift_grammar.md. Types and patterns are
// first-class because the grammar makes them so: a type is written
// where a type goes and nowhere else, and a pattern is what binds
// names in a declaration, a for-in, a case label, and a catch clause
// alike.
//
// Invariants:
//
//   - Every node embeds a Span. Pos and End are stored, not derived,
//     so even error-recovery nodes have a real, non-empty extent.
//   - Nodes hold no text. An Ident is two positions; a literal is two
//     positions and a token.Kind. Decoding — a number's value, a
//     string's escapes, a multiline literal's indentation — belongs
//     to phases above this one. Anything reading spelling takes the
//     *token.File.
//   - Nothing here is resolved. An operator is a span, not a
//     precedence: what `a + b * c` groups into is decided by the
//     precedencegroup declarations the analyzer collects, so the
//     parser leaves a flat SequenceExpr and the analyzer folds it.
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

// Ident is two positions; spelling resolves through the File that
// produced it. It is not an Expr: an identifier in expression
// position is an IdentExpr, which may also carry a generic argument
// clause.
type Ident struct {
	Span
	// Escaped records the `backtick` spelling, whose backticks are
	// part of the span but not of the name.
	Escaped bool
}

// Name returns the identifier's spelling, backticks included.
func (id *Ident) Name(f *token.File) string {
	if id == nil {
		return ""
	}
	return string(f.Slice(id.Lo, id.Hi))
}

// Text returns the identifier's name with any backticks stripped —
// what the name denotes, rather than how it was written.
func (id *Ident) Text(f *token.File) string {
	s := id.Name(f)
	if id != nil && id.Escaped && len(s) >= 2 {
		return s[1 : len(s)-1]
	}
	return s
}

// Releaser is the one-method window through which ast sees the
// parser's arena.
type Releaser interface {
	Release()
}

// File is one source file's tree: the grammar's TopLevelDeclaration,
// which is a statement list — a declaration is a statement, so a file
// holds both, in written order.
type File struct {
	Span
	Unit     *token.File   // the position space every span resolves through
	Stmts    []Stmt        // the top level, in written order
	Comments []token.Token // retained under parser.ParseComments

	rel Releaser
}

// SetReleaser attaches the tree's backing storage. The parser calls
// this; consumers call Release.
func (f *File) SetReleaser(r Releaser) { f.rel = r }

// Release returns the tree's backing storage. It is safe on a tree
// with no releaser and safe to call twice, but every node is invalid
// afterwards — copy what you need (usually a span and a string)
// before calling it. Release is a promise, not a check: nothing
// detects a kept pointer.
func (f *File) Release() {
	if f.rel != nil {
		r := f.rel
		f.rel = nil
		r.Release()
	}
}

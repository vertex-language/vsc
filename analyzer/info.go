package analyzer

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// Info holds the results of semantic analysis for a parsed unit or package.
type Info struct {
	// Types maps each evaluated expression to its resolved semantic type.
	Types map[ast.Expr]types.Type

	// Defs maps each identifier that defines a symbol to that symbol.
	Defs map[*ast.Ident]Symbol

	// Uses maps each identifier that refers to a symbol to that symbol.
	Uses map[*ast.Ident]Symbol

	// Scopes maps AST nodes (File, CodeBlock, StructDecl, etc.) to their lexical scope.
	Scopes map[ast.Node]*Scope

	// Folded maps each flat SequenceExpr to its precedence-folded expression tree.
	Folded map[*ast.SequenceExpr]ast.Expr

	// Operators maps each operator expression to the declaration it
	// resolved to. An operator is a function, and this is the same
	// answer Uses gives for a call — kept apart only because an
	// operator is not written as a name.
	Operators map[ast.Expr]Symbol

	// Values maps each literal to the value it denotes: a BasicLit or
	// a StringLit with nothing to interpolate, and every run of text
	// inside an interpolated one, so that a consumer reads the pieces
	// of `"a\(b)c"` in order.
	Values map[ast.Node]Value

	// Methods maps each member expression that named a method to the
	// method and the type that declares it.
	//
	// The type is half the answer, not decoration: a method's symbol is
	// mangled inside the nominal it belongs to, so a consumer that knew
	// only the signature could not name the thing it wants to call.
	// Uses cannot carry it — that maps a name to a symbol, and a method
	// lives in its type's scope rather than in the one the call was
	// written in.
	Methods map[*ast.MemberExpr]*MethodRef

	// Extensions is the type each extension extends. The resolution
	// happens in the checker and is cached privately there, but
	// lowering needs it too: an extension's methods are emitted with
	// the extended type as their receiver, and the syntax only names
	// the type rather than pointing at it.
	Extensions map[*ast.ExtensionDecl]types.Type

	// Diagnostics holds all warnings and errors produced during analysis.
	Diagnostics []token.Diagnostic
}

// A MethodRef is a method and the nominal type it was found in.
//
// Recv is where the lookup ended rather than where it started: a method
// inherited from a superclass belongs to the superclass, and that is the
// type its symbol names.
type MethodRef struct {
	Recv   types.Type
	Method *types.Method
}

// NewInfo allocates an empty Info container.
func NewInfo() *Info {
	return &Info{
		Types:       make(map[ast.Expr]types.Type),
		Defs:        make(map[*ast.Ident]Symbol),
		Uses:        make(map[*ast.Ident]Symbol),
		Scopes:      make(map[ast.Node]*Scope),
		Folded:      make(map[*ast.SequenceExpr]ast.Expr),
		Operators:   make(map[ast.Expr]Symbol),
		Values:      make(map[ast.Node]Value),
		Methods:     make(map[*ast.MemberExpr]*MethodRef),
		Extensions:  make(map[*ast.ExtensionDecl]types.Type),
		Diagnostics: nil,
	}
}

// TypeOf returns the type of expression e, or nil if unrecorded.
func (info *Info) TypeOf(e ast.Expr) types.Type {
	if info.Types == nil {
		return nil
	}
	return info.Types[e]
}

// SymbolOf returns the symbol defined or used by ident, or nil.
func (info *Info) SymbolOf(id *ast.Ident) Symbol {
	if s, ok := info.Defs[id]; ok {
		return s
	}
	if s, ok := info.Uses[id]; ok {
		return s
	}
	return nil
}

// ScopeOf returns the Scope associated with node n, or nil.
func (info *Info) ScopeOf(n ast.Node) *Scope {
	if info.Scopes == nil {
		return nil
	}
	return info.Scopes[n]
}

// FoldedOf returns the folded tree for a SequenceExpr, or the sequence itself if not folded.
func (info *Info) FoldedOf(seq *ast.SequenceExpr) ast.Expr {
	if f, ok := info.Folded[seq]; ok {
		return f
	}
	return seq
}

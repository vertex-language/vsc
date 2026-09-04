// Package analyzer reads a parsed program: it resolves names, folds
// the expression sequences the parser left flat, gives every
// expression a type, and decodes what every literal says.
//
// The passes run in order, over every file at once, because a Swift
// program is not read top to bottom: a function may call one declared
// below it, and a type may be used before the line that declares it.
// So the names come first — precedence groups and operators, then
// nominal types, then their members, then extensions, then functions
// — and only when all of them are known are the bodies checked.
//
// The rule the whole package is built on:
//
//	Where the checker does not know, it says nothing. It never
//	invents an answer.
//
// An invented type is worse than no type. A parser that rejects valid
// Swift fails loudly and someone fixes it; a checker that answers Int
// where it cannot work out the answer hands the phase below it a
// well-typed module describing a program nobody wrote. So a type this
// package cannot read is Invalid, a diagnostic whose subject is
// Invalid is not reported, and one mistake in the source is one
// diagnostic in the output.
//
// What is modelled and what is passed over in silence is written down
// in README.md, and tests/check is where both halves are held to
// Swift's own verdicts.
package analyzer

import (
	"fmt"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

type checker struct {
	file        *token.File
	pg          *PrecedenceGraph
	info        *Info
	resolved    map[ast.Type]types.Type
	currFuncRet types.Type
	currType    types.Type // the type whose members are being checked
	negated     map[ast.Expr]bool
	typeScopes  map[string]*Scope // a declared type's scope, by name
	currActor   *types.Class
	inAwait     bool
}

// typeErrorf reports a diagnostic about types, unless one of them
// failed to resolve. That mistake was reported where it was made, and
// what follows from it says nothing the reader does not know.
func (c *checker) typeErrorf(pos token.Pos, format string, args ...any) {
	for _, a := range args {
		if t, ok := a.(types.Type); ok && isInvalid(t) {
			return
		}
	}
	c.errorf(pos, format, args...)
}

// isInvalid reports whether t is the placeholder a type that did not
// resolve leaves behind.
func isInvalid(t types.Type) bool {
	b, ok := t.(*types.Basic)
	return ok && b.Kind() == types.Invalid
}

func (c *checker) errorf(pos token.Pos, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	diag := token.Diagnostic{
		Pos:      pos,
		End:      pos,
		Severity: token.Error,
		Message:  msg,
	}
	c.info.Diagnostics = append(c.info.Diagnostics, diag)
}

// Check performs full semantic analysis, precedence folding, and type checking
// on the given AST files.
func Check(files []*ast.File) (*Info, []token.Diagnostic) {
	info := NewInfo()
	pg := NewPrecedenceGraph()

	// 1. Root Universe Scope
	universeScope := NewScope(nil, token.NoPos, token.NoPos)
	for name, typ := range types.Typ {
		if typ != nil && typ.Name() != "" {
			universeScope.Insert(NewTypeName(typ.Name(), typ, token.NoPos))
		}
		_ = name
	}

	// 2. Package Scope
	pkgScope := NewScope(universeScope, token.NoPos, token.NoPos)

	c := &checker{
		pg:      pg,
		info:    info,
		negated: make(map[ast.Expr]bool),
	}

	// Multi-pass analysis over all compilation units:

	// Pass 1: Precedence groups and custom operators
	for _, f := range files {
		if f.Unit != nil {
			c.file = f.Unit
		}
		c.declarePrecedenceAndOperators(declsOf(f.Stmts))
	}

	// Pass 2: Nominal types (structs, classes, enums, protocols, typealiases)
	for _, f := range files {
		if f.Unit != nil {
			c.file = f.Unit
		}
		c.declareTypes(declsOf(f.Stmts), pkgScope)
	}

	// Pass 3: Type members, fields, enum cases, superclasses
	for _, f := range files {
		if f.Unit != nil {
			c.file = f.Unit
		}
		c.resolveTypeMembers(declsOf(f.Stmts), pkgScope)
	}

	// Pass 3.5: Extensions
	for _, f := range files {
		if f.Unit != nil {
			c.file = f.Unit
		}
		c.resolveExtensions(declsOf(f.Stmts), pkgScope)
	}

	// Pass 4: Top-level function declarations
	for _, f := range files {
		if f.Unit != nil {
			c.file = f.Unit
		}
		c.declareFunctions(declsOf(f.Stmts), pkgScope)
	}

	// Pass 4.5: Validate protocol conformances
	c.checkProtocolConformances(pkgScope)

	// Pass 5: Type-check all top-level statements and bodies
	for _, f := range files {
		if f.Unit != nil {
			c.file = f.Unit
		}
		info.Scopes[f] = pkgScope
		for _, stmt := range f.Stmts {
			c.checkStmt(stmt, pkgScope)
		}
	}

	token.SortDiagnostics(info.Diagnostics)
	return info, info.Diagnostics
}

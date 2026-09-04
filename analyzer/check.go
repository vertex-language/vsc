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

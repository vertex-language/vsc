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

	// declSites is where each top-level declaration was written and
	// how far it can be seen from. See access.go.
	declSites  map[Symbol]declSite
	typeScopes map[string]*Scope // a declared type's scope, by name
	currActor  *types.Class
	inAwait    bool
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
	return CheckImporting(files, nil)
}

// An Import is another module's public interface, already parsed.
//
// The interface is source -- Swift's answer, and the reason this
// needs no second parser -- so what arrives here is what arrives for
// the program itself: files and the tables their positions are
// measured against.
type Import struct {
	Name  string
	Files []*ast.File
	Units []*token.File
}

// CheckImporting checks files as a module that can see imports.
//
// An imported module's declarations go in a scope between the
// built-in one and the program's own, which is what makes a local
// declaration shadow an imported one of the same name rather than
// collide with it.
//
// Their bodies are not checked: an interface has none. What is taken
// from them is what a client needs -- the signatures, the stored
// properties in order, the methods in order, the cases in order --
// and every symbol is recorded as belonging to the module it came
// from, because that is what its symbol is mangled with.
func CheckImporting(files []*ast.File, imports []Import) (*Info, []token.Diagnostic) {
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

	// 2. The built-in module, between the universe and the program.
	//
	// Its declarations are read the way any file's are, into a scope
	// of their own: a program sees them, and nothing it declares can
	// be confused with them. What it says about itself — a
	// diagnostic in core.swift — is not the caller's business and is
	// dropped; core has a test of its own for that.
	coreScope := NewScope(universeScope, token.NoPos, token.NoPos)

	// 3. The imported modules, between the built-ins and the program.
	importScope := NewScope(coreScope, token.NoPos, token.NoPos)

	// 4. Package Scope
	pkgScope := NewScope(importScope, token.NoPos, token.NoPos)

	c := &checker{
		pg:        pg,
		info:      info,
		negated:   make(map[ast.Expr]bool),
		declSites: make(map[Symbol]declSite),
	}
	c.loadCore(coreScope)
	c.loadImports(imports, importScope)

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

// loadImports declares what the imported modules export.
//
// The same passes the program gets, in the same order and over the
// same kind of input -- an interface is source -- but stopping before
// the one that checks bodies, because an interface has none. What
// each pass leaves behind is what a client needs: a struct's fields
// in declaration order, which is its layout; a class's methods in
// declaration order, which is its vtable; an enum's cases in
// declaration order, which are its tags.
func (c *checker) loadImports(imports []Import, scope *Scope) {
	if len(imports) == 0 {
		return
	}
	// An interface says nothing about the client, so what it says
	// about itself is not the client's business. A broken interface
	// is the fault of whoever built the module it describes, and is
	// reported when that module is built.
	quiet := len(c.info.Diagnostics)
	defer func() { c.info.Diagnostics = c.info.Diagnostics[:quiet] }()

	prev := c.file
	defer func() { c.file = prev }()

	for _, imp := range imports {
		for i, f := range imp.Files {
			if i < len(imp.Units) {
				c.file = imp.Units[i]
			} else if f.Unit != nil {
				c.file = f.Unit
			}
			decls := declsOf(f.Stmts)
			c.declarePrecedenceAndOperators(decls)
			c.declareTypes(decls, scope)
		}
		for i, f := range imp.Files {
			if i < len(imp.Units) {
				c.file = imp.Units[i]
			}
			decls := declsOf(f.Stmts)
			c.resolveTypeMembers(decls, scope)
		}
		for i, f := range imp.Files {
			if i < len(imp.Units) {
				c.file = imp.Units[i]
			}
			decls := declsOf(f.Stmts)
			c.resolveExtensions(decls, scope)
			c.declareFunctions(decls, scope)
		}
		c.recordModule(imp, scope)
	}
}

// recordModule marks everything an import declared as belonging to
// it, so that lowering mangles those symbols with their own module's
// name rather than with the one being compiled.
func (c *checker) recordModule(imp Import, scope *Scope) {
	for _, sym := range scope.Symbols() {
		if _, already := c.info.Imported[sym]; already {
			continue
		}
		c.info.Imported[sym] = imp.Name
		if tn, ok := sym.(*TypeNameSymbol); ok && tn.Type() != nil {
			// Under both spellings: a lookup may arrive with the name
			// or with what the name stands for.
			c.info.ImportedTypes[tn.Type()] = imp.Name
			c.info.ImportedTypes[tn.Type().Underlying()] = imp.Name
		}
	}
}

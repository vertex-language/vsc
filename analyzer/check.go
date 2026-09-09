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
	// modules is what each module name may be followed by. See
	// resolveMemberType: `Swift.Int` is a name in a module, and an
	// interface writes every name that way.
	modules   map[string]*Scope
	currActor *types.Class
	inAwait   bool
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
		// The file being read, so that a diagnostic about an imported
		// declaration is rendered against the interface it is in
		// rather than against whichever file the caller happens to
		// hand the printer.
		File: c.file,
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
	// Name is the module's own name: what its symbols are mangled
	// with, and what an interface of its own would declare.
	Name string
	// As is the name this program refers to it by, which differs from
	// Name only where an import renamed it. Empty means Name.
	//
	// The two are separate because a rename is about the reference
	// and not about the module: renaming does not change a symbol, so
	// two modules that share a Name still collide at the link however
	// the importing file spells them.
	As    string
	Files []*ast.File
	Units []*token.File
}

// bound is the name this program looks the module up under.
func (i Import) bound() string {
	if i.As != "" {
		return i.As
	}
	return i.Name
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
	// Vertex's lowercase spellings, under their own names and denoting
	// the same types. They belong in the scope rather than only in
	// LookupUniverse: a name with no symbol is a type in type position
	// and nothing in expression position, so `int32(x)` was a
	// constructor call with no type behind it while `Int32(x)` was a
	// conversion. Insert keeps the first of a name, so a Swift name
	// this happens to share stays Swift's.
	for name, typ := range types.VertexAliases() {
		if typ != nil {
			universeScope.Insert(NewTypeName(name, typ, token.NoPos))
		}
	}
	// The pointers, for the same reason said the same way:
	// `UnsafeRawPointer(p)` is a conversion in expression position,
	// and without a symbol the name is a type in type position and
	// nothing here.
	for name, typ := range types.PointerNames() {
		universeScope.Insert(NewTypeName(name, typ, token.NoPos))
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
		modules:   map[string]*Scope{},
	}
	c.loadCore(coreScope)
	// Swift is the module the built-ins are from, and a program that
	// writes `Swift.Int` is naming the Int it already has. Nothing
	// declares that module here -- there is no interface for it to
	// read -- so it is the scope those types were put in.
	c.modules["Swift"] = coreScope
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

	// Pass 3.6: receiver methods, which are extension members written
	// the other way round. After the extensions, so that a receiver
	// method may name a type an extension is also adding to.
	for _, f := range files {
		if f.Unit != nil {
			c.file = f.Unit
		}
		c.resolveReceivers(declsOf(f.Stmts), pkgScope)
	}

	// Pass 3.75: what each type chose for the associated types its
	// protocols name. After the extensions, because an extension may
	// be where the method that implies the choice was written.
	for _, f := range files {
		if f.Unit != nil {
			c.file = f.Unit
		}
		c.resolveAssociatedTypes(declsOf(f.Stmts), pkgScope)
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
		// The module's name means something while its own interface
		// is being read, because an interface names its own types
		// through itself: swiftc writes `func pointSum(_ p: Lib.Point)`
		// in Lib's own interface. Until the module has a scope of its
		// own -- which recordModule gives it once every declaration is
		// in -- what is in scope is what has been read so far, which
		// is where those names are.
		//
		// Each module is read into a staging scope of its own rather
		// than straight into the shared one. Scope.Insert keeps the
		// first symbol of a name, so two modules that both export
		// `width` used to leave the second one's nowhere at all --
		// not in the shared scope, and so not in its own module's
		// either, which made `B.width` unresolvable. Staging is
		// chained to the shared scope so an interface can still name
		// what modules read before it declared.
		staging := NewScope(scope, token.NoPos, token.NoPos)
		if c.modules[imp.bound()] == nil {
			c.modules[imp.bound()] = staging
		}
		for i, f := range imp.Files {
			if i < len(imp.Units) {
				c.file = imp.Units[i]
			} else if f.Unit != nil {
				c.file = f.Unit
			}
			decls := declsOf(f.Stmts)
			c.declarePrecedenceAndOperators(decls)
			c.declareTypes(decls, staging)
		}
		for i, f := range imp.Files {
			if i < len(imp.Units) {
				c.file = imp.Units[i]
			}
			decls := declsOf(f.Stmts)
			c.resolveTypeMembers(decls, staging)
		}
		for i, f := range imp.Files {
			if i < len(imp.Units) {
				c.file = imp.Units[i]
			}
			decls := declsOf(f.Stmts)
			c.resolveExtensions(decls, staging)
			c.resolveAssociatedTypes(decls, staging)
			c.declareFunctions(decls, staging)
		}
		c.recordModule(imp, staging, scope)
	}
}

// recordModule marks everything an import declared as belonging to
// it, so that lowering mangles those symbols with their own module's
// name rather than with the one being compiled.
// It also collects them into a scope of the module's own, which is
// what a qualified name is looked up in: every import shares one
// scope so that an unqualified name finds them all, and `Foundation.Data`
// has to be able to mean Foundation's and not somebody else's.
func (c *checker) recordModule(imp Import, staging, scope *Scope) {
	// The module's own scope has no parent: a qualified name is
	// exact, and must not fall through to what another module
	// declared under the same name.
	own := NewScope(nil, token.NoPos, token.NoPos)
	c.modules[imp.bound()] = own

	for _, sym := range staging.Symbols() {
		own.Insert(sym)
		if _, already := c.info.Imported[sym]; !already {
			c.info.Imported[sym] = imp.Name
		}
		// The shared scope keeps the first of a name, which is what
		// an unqualified reference finds. The module keeps all of
		// them, which is what a qualified one needs.
		scope.Insert(sym)
		if tn, ok := sym.(*TypeNameSymbol); ok && tn.Type() != nil {
			// Under both spellings: a lookup may arrive with the name
			// or with what the name stands for.
			if _, already := c.info.ImportedTypes[tn.Type()]; !already {
				c.info.ImportedTypes[tn.Type()] = imp.Name
				c.info.ImportedTypes[tn.Type().Underlying()] = imp.Name
			}
		}
	}
}

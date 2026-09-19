package analyzer

import (
	"bytes"
	"fmt"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

type checker struct {
	// initLog is the deferred lets given their value since the branch
	// being checked began; see branch.
	initLog     []*VarSymbol
	file        *token.File
	pg          *PrecedenceGraph
	info        *Info
	resolved    map[ast.Type]types.Type
	currFuncRet types.Type
	// currFuncName is what #function says where it is written: the
	// declaration being checked, spelled as Swift names it.
	currFuncName string
	// inPropertyInit is whether a stored property's initializer is being
	// checked, where there is no self yet to reach an instance member through.
	inPropertyInit bool
	currType     types.Type // the type whose members are being checked
	negated      map[ast.Expr]bool

	// stored is the bindings already declared, so that a module-scope
	// variable declared ahead of the bodies is not declared again when
	// the statement it came from is reached. See declareModuleVars.
	stored map[*ast.PatternBinding]*Scope
	// implicit is the expressions whose value is an implicitly unwrapped
	// optional taken as an optional; see implicitlyUnwrapped.
	implicit map[ast.Expr]bool

	// declSites tracks top-level declaration locations and access levels.
	declSites  map[Symbol]declSite
	typeScopes map[string]*Scope // a declared type's scope, by name
	// modules maps module names to their scopes for qualified lookup.
	modules   map[string]*Scope
	currActor *types.Class
	inAwait   bool
	// currAsync is whether the function or closure being checked is
	// async, which decides between overloads that differ only in that.
	currAsync bool
	// inInit indicates an initializer body is being checked.
	inInit bool
	// inChain is the steps of optional chains below their roots.
	inChain map[ast.Expr]bool
	// importing is the module whose interface is being read, or "".
	importing string
}

// typeErrorf reports a type diagnostic unless one of the types is types.Invalid.
func (c *checker) typeErrorf(pos token.Pos, format string, args ...any) {
	for _, a := range args {
		if t, ok := a.(types.Type); ok && isInvalid(t) {
			return
		}
	}
	c.errorf(pos, format, args...)
}

// isInvalid reports whether t is types.Invalid.
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
		File:     c.file,
	}
	c.info.Diagnostics = append(c.info.Diagnostics, diag)
}

// Check performs full semantic analysis, precedence folding, and type checking
// on the given AST files.
func Check(files []*ast.File) (*Info, []token.Diagnostic) {
	return CheckImporting(files, nil)
}

// An Import represents another module's public interface AST files.
type Import struct {
	Name  string // module's declared name
	As    string // local alias if renamed upon import, or empty
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

// CheckImporting checks files as a module that can see imported modules.
func CheckImporting(files []*ast.File, imports []Import) (*Info, []token.Diagnostic) {
	return CheckModule("", files, imports)
}

// CheckModule is CheckImporting for a module that knows its own name,
// which its code may use to qualify its own declarations --
// `tls.ConnectionState()` inside module tls -- as Swift allows.
func CheckModule(module string, files []*ast.File, imports []Import) (*Info, []token.Diagnostic) {
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
	// Vertex lowercase aliases and pointer types.
	for name, typ := range types.VertexAliases() {
		if typ != nil {
			universeScope.Insert(NewTypeName(name, typ, token.NoPos))
		}
	}
	for name, typ := range types.PointerNames() {
		universeScope.Insert(NewTypeName(name, typ, token.NoPos))
	}

	// 2. Built-in module scope, between universe and user code.
	coreScope := NewScope(universeScope, token.NoPos, token.NoPos)

	// 3. Imported modules scope.
	importScope := NewScope(coreScope, token.NoPos, token.NoPos)

	// 4. Package Scope
	pkgScope := NewScope(importScope, token.NoPos, token.NoPos)

	c := &checker{
		pg:        pg,
		info:      info,
		negated:   make(map[ast.Expr]bool),
		implicit:  make(map[ast.Expr]bool),
		declSites: make(map[Symbol]declSite),
		modules:   map[string]*Scope{},
	}
	c.loadCore(coreScope)
	c.modules["Swift"] = coreScope
	c.loadImports(imports, importScope)
	// A module may name itself. Its own declarations are what the
	// package scope holds, which is looked up under the name where
	// nothing in scope has it.
	if module != "" {
		if _, taken := c.modules[module]; !taken {
			c.modules[module] = pkgScope
		}
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

	// Pass 3.6: Receiver methods
	for _, f := range files {
		if f.Unit != nil {
			c.file = f.Unit
		}
		c.resolveReceivers(declsOf(f.Stmts), pkgScope)
	}

	// Pass 3.75: Associated types
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

	// Pass 4.6: Module-scope variables, so that one file's `let` is in
	// scope in another whatever order the two were read in.
	//
	// Only for a file that is declarations and nothing else. A file with
	// top-level code in it runs that code in the order it is written,
	// and a variable there is initialized where it is written like any
	// other statement -- declaring it ahead of them would be saying it
	// held its value before the line that gives it one. Swift draws the
	// same line and draws it at the file: top-level code lives in one
	// file, and a variable anywhere else is order-independent.
	for _, f := range files {
		if f.Unit != nil {
			c.file = f.Unit
		}
		if !declarationsOnly(f.Stmts) {
			continue
		}
		c.declareModuleVars(declsOf(f.Stmts), pkgScope)
	}

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

	defer func() { c.importing = "" }()
	for _, imp := range imports {
		c.importing = imp.Name
		// Stage declarations into a module-specific scope before merging into shared scope.
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
			c.resolveReceivers(decls, staging)
			c.resolveAssociatedTypes(decls, staging)
			c.declareFunctions(decls, staging)
		}
		c.recordModule(imp, staging, scope)
	}
}

// recordModule attributes declarations to their defining module for mangling and qualified lookup.
func (c *checker) recordModule(imp Import, staging, scope *Scope) {
	own := NewScope(nil, token.NoPos, token.NoPos)
	c.modules[imp.bound()] = own
	for _, u := range imp.Units {
		if u != nil && bytes.HasPrefix(u.Text(), []byte("// swift-interface-format-version")) {
			c.info.SwiftModules[imp.Name] = true
		}
	}

	unitOf := map[ast.Decl]*token.File{}
	for i, f := range imp.Files {
		if i >= len(imp.Units) {
			break
		}
		for _, st := range f.Stmts {
			if ds, ok := st.(*ast.DeclStmt); ok {
				unitOf[ds.D] = imp.Units[i]
			}
		}
	}
	for _, sym := range staging.Symbols() {
		own.Insert(sym)
		if _, already := c.info.Imported[sym]; !already {
			c.info.Imported[sym] = imp.Name
			if fn, ok := sym.(*FuncSymbol); ok {
				if d, ok := fn.Decl().(ast.Decl); ok && unitOf[d] != nil {
					c.info.ImportedUnits[sym] = unitOf[d]
				}
			}
		}
		scope.Insert(sym)
		if tn, ok := sym.(*TypeNameSymbol); ok && tn.Type() != nil {
			if _, already := c.info.ImportedTypes[tn.Type()]; !already {
				c.info.ImportedTypes[tn.Type()] = imp.Name
				c.info.ImportedTypes[tn.Type().Underlying()] = imp.Name
			}
		}
	}
}

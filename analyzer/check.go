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
	// branchValues is the expression statements that are the branches of
	// an if or a switch used as a value, and the type each is wanted as.
	branchValues map[*ast.ExprStmt]types.Type
	// currFuncName is what #function says where it is written: the
	// declaration being checked, spelled as Swift names it.
	currFuncName string
	// inPropertyInit is whether a stored property's initializer is being
	// checked, where there is no self yet to reach an instance member through.
	inPropertyInit bool
	currType       types.Type // the type whose members are being checked
	negated        map[ast.Expr]bool

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
	// scopesByType is the same, by the type: two imported modules may
	// each declare a Cursor, and a name keeps only the last of them.
	scopesByType map[types.Type]*Scope
	// modules maps module names to their scopes for qualified lookup.
	modules   map[string]*Scope
	currActor *types.Class
	inAwait   bool
	// currAsync is whether the function or closure being checked is
	// async, which decides between overloads that differ only in that.
	currAsync bool
	// currThrown is the error type the function being checked declares
	// with `throws(E)`: what `throw .refused` is read as. Nil where the
	// function throws any Error, or inside a do body.
	currThrown types.Type
	// inferRet collects what the returns of a closure whose result is
	// still to be inferred give, where it has more than one statement.
	inferRet *[]types.Type
	// builtClosures are the closures a result builder has rewritten.
	builtClosures map[*ast.ClosureExpr]bool
	// files are the module's files; packExpansions the expansions made of
	// each function with parameter packs, by name. See packs.go.
	files          []*ast.File
	packExpansions map[*ast.FuncDecl]map[string]bool
	// callees are the expressions called: `x.m` before `(...)` is a call
	// of m, and anywhere else a reference to it. See refs.go.
	callees map[ast.Expr]bool
	// asyncLets are the names `async let` bound, each holding the Task
	// its initializer runs as; asyncLetReads are the reads made of them
	// that read the Task itself. See rewriteAsyncLet.
	asyncLets     map[*VarSymbol]bool
	asyncLetReads map[*ast.IdentExpr]bool
	// currIsolated is whether the code being checked runs on the main
	// thread: a @MainActor function or type's member, a closure made
	// there, or top-level code. See isolation.go.
	currIsolated bool
	// memberIsolated is whether the members being read belong to a
	// @MainActor type, which makes each of them @MainActor.
	memberIsolated bool
	// assigning is the member being written by the assignment under
	// check, whose read rules do not apply to it.
	assigning ast.Expr
	// pkgScope is the module's own scope: where a top-level function is
	// declared, as against one local to a body.
	pkgScope *Scope
	// inInit indicates an initializer body is being checked.
	inInit bool
	// inChain is the steps of optional chains below their roots.
	inChain map[ast.Expr]bool
	// importing is the module whose interface is being read, or "".
	importing string
	// wrapped is the properties property wrappers give, finished once
	// every type's members are read.
	wrapped []wrappedProperty
	// checkedEarly is the functions returning `some P` whose bodies were
	// checked before the rest, so that callers anywhere know the type.
	checkedEarly map[*ast.FuncDecl]bool
	// opaqueParams is the generic parameter each `some P` parameter type is.
	opaqueParams map[ast.Type]*types.TypeParam
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
	types.BuiltinAssoc = info.builtinAssoc

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
		files:     files,
		pg:        pg,
		info:      info,
		negated:   make(map[ast.Expr]bool),
		implicit:  make(map[ast.Expr]bool),
		declSites: make(map[Symbol]declSite),
		modules:   map[string]*Scope{},
	}
	c.pkgScope = pkgScope
	c.loadCore(coreScope)
	c.modules["Swift"] = coreScope
	c.loadAlgorithms(coreScope)
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

	// Pass 3.52: What property wrappers give the properties they wrap.
	c.finishWrappedProperties()

	// Pass 3.55: Initializers a subclass inherits.
	for _, f := range files {
		if f.Unit != nil {
			c.file = f.Unit
		}
		c.inheritInitializers(declsOf(f.Stmts), pkgScope)
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

	// Pass 4.4: Derived conformances. Every conformance is known now, and
	// none has been checked for its witnesses: what a type gets by
	// conforming is made here, and checked with the module from here on.
	if derived := c.deriveConformances(files, pkgScope); derived != nil {
		c.info.Derived = derived
		c.info.DerivedText = derived.Unit.Text()
		files = append(files[:len(files):len(files)], derived)
		c.files = files
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

	// Pass 4.9: The functions returning `some P`, whose bodies say what
	// their callers get. One whose body does not check here yet -- it
	// reads what top-level code has still to declare -- waits its turn.
	for _, f := range files {
		if f.Unit != nil {
			c.file = f.Unit
		}
		c.currIsolated = !declarationsOnly(f.Stmts)
		for _, stmt := range f.Stmts {
			ds, ok := stmt.(*ast.DeclStmt)
			if !ok {
				continue
			}
			fd, ok := ds.D.(*ast.FuncDecl)
			if !ok || fd.Recv != nil || fd.Sig == nil || fd.Sig.Result == nil {
				continue
			}
			if _, opaque := fd.Sig.Result.Type.(*ast.OpaqueType); !opaque {
				continue
			}
			quiet := len(c.info.Diagnostics)
			c.checkStmt(stmt, pkgScope)
			if len(c.info.Diagnostics) > quiet {
				c.info.Diagnostics = c.info.Diagnostics[:quiet]
				continue
			}
			if c.checkedEarly == nil {
				c.checkedEarly = map[*ast.FuncDecl]bool{}
			}
			c.checkedEarly[fd] = true
		}
	}

	// Pass 5: Type-check all top-level statements and bodies. Top-level
	// code runs on the main thread, as Swift's does.
	c.currIsolated = true
	for _, f := range files {
		if f.Unit != nil {
			c.file = f.Unit
		}
		info.Scopes[f] = pkgScope
		// Top-level code that awaits is an async context: main.swift's
		// statements run as the program's first task.
		c.currAsync = TopLevelAwaits(f)
		if c.currAsync {
			info.AsyncTopLevel = true
		}
		for _, stmt := range f.Stmts {
			c.checkStmt(stmt, pkgScope)
		}
		c.currAsync = false
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
		// What its types get by conforming, where it does not say already:
		// a source package's, or an interface's written before it did.
		for i, f := range imp.Files {
			if f.Unit == nil && i < len(imp.Units) {
				f.Unit = imp.Units[i]
			}
		}
		if derived := c.deriveConformances(imp.Files, staging); derived != nil {
			imp.Files = append(imp.Files[:len(imp.Files):len(imp.Files)], derived)
			imp.Units = append(imp.Units[:len(imp.Units):len(imp.Units)], derived.Unit)
		}
		// Stored module-scope variables, last, so that an initializer
		// whose type has to be inferred sees every function it may call.
		// An annotated one is declared from its annotation alone: its
		// initializer runs in its own module, and is not this one's to check.
		for i, f := range imp.Files {
			if i < len(imp.Units) {
				c.file = imp.Units[i]
			}
			c.declareImportedVars(declsOf(f.Stmts), staging)
		}
		// The generic declarations' bodies, which a client specializes and
		// so has to have checked. What they say is the module's own
		// business -- it was said when the module was built -- so their
		// diagnostics go with the rest of the import's.
		for i, f := range imp.Files {
			if i < len(imp.Units) {
				c.file = imp.Units[i]
				if f.Unit == nil {
					f.Unit = imp.Units[i]
				}
			}
			c.checkImportedGenerics(declsOf(f.Stmts), staging)
			c.info.ImportedFiles = append(c.info.ImportedFiles, f)
		}
		c.recordModule(imp, staging, scope)
	}
}

// checkImportedGenerics checks the bodies of an imported module's generic
// declarations: generic functions, generic types (all their members), and
// extensions and receiver methods of generic types. Nothing else of an
// import is checked -- the rest is called, not compiled, here.
func (c *checker) checkImportedGenerics(decls []ast.Decl, scope *Scope) {
	for _, decl := range decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Body == nil {
				continue
			}
			if d.Generics != nil || (d.Recv != nil && c.namesGenericType(d.Recv.Type, scope)) {
				c.checkDecl(d, scope)
			}
		case *ast.StructDecl:
			if d.Generics != nil {
				c.checkDecl(d, scope)
			}
		case *ast.ClassDecl:
			if d.Generics != nil {
				c.checkDecl(d, scope)
			}
		case *ast.EnumDecl:
			if d.Generics != nil {
				c.checkDecl(d, scope)
			}
		case *ast.ExtensionDecl:
			if c.namesGenericType(d.Type, scope) {
				c.checkDecl(d, scope)
			}
		}
	}
}

// namesGenericType reports whether a written type names a nominal type
// that has type parameters of its own.
func (c *checker) namesGenericType(t ast.Type, scope *Scope) bool {
	if t == nil {
		return false
	}
	quiet := len(c.info.Diagnostics)
	resolved := c.resolveType(t, scope)
	c.info.Diagnostics = c.info.Diagnostics[:quiet]
	if resolved == nil {
		return false
	}
	if gi, ok := resolved.(*types.GenericInstance); ok {
		resolved = gi.Base
	}
	switch u := resolved.Underlying().(type) {
	case *types.Struct:
		return len(u.TypeParams) > 0
	case *types.Class:
		return len(u.TypeParams) > 0
	case *types.Enum:
		return len(u.TypeParams) > 0
	}
	return false
}

// declareImportedVars declares the stored module-scope variables an
// imported module's files declare, as the client sees them.
func (c *checker) declareImportedVars(decls []ast.Decl, scope *Scope) {
	for _, decl := range decls {
		d, ok := decl.(*ast.VarDecl)
		if !ok {
			continue
		}
		for _, b := range d.Bindings {
			if b.Body != nil || b.Accessors != nil {
				continue
			}
			if _, typed := b.Pat.(*ast.TypedPattern); typed {
				c.declarePatternInit(b.Pat, nil, d.Kind == token.LET, true, scope)
				continue
			}
			c.checkStored(b, d.Kind == token.LET, scope)
		}
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

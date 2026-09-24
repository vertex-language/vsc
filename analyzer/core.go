package analyzer

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// loadCore loads the built-in core module declarations into scope.
func (c *checker) loadCore(scope *Scope) {
	file, unit, diags := core.Files()
	if file == nil || len(diags) > 0 {
		return
	}

	prevFile := c.file
	prevDiags := len(c.info.Diagnostics)
	c.file = unit

	decls := declsOf(file.Stmts)
	c.declarePrecedenceAndOperators(decls)
	c.declareTypes(decls, scope)
	c.resolveTypeMembers(decls, scope)
	c.declareFunctions(decls, scope)

	// Mark core symbols as imported from Swift standard library.
	for _, sym := range scope.Symbols() {
		if fn, ok := sym.(*FuncSymbol); ok {
			c.info.Imported[fn] = "Swift"
		}
		if tn, ok := sym.(*TypeNameSymbol); ok && tn.Type() != nil {
			c.info.CoreTypes[tn.Type()] = true
		}
	}

	c.info.Diagnostics = c.info.Diagnostics[:prevDiags]
	c.file = prevFile
}

// loadAlgorithms reads the core's algorithms, which are source with
// bodies -- declared and checked as a program's own declarations are,
// in the core's scope, and attributed to the core as its extensions'
// members. A program lowers what it uses of them.
//
// What is wrong with the core is the core's fault, not the program's,
// and is found by the core's own test: the program sees nothing of it.
func (c *checker) loadAlgorithms(scope *Scope) {
	file, unit, diags := core.Algorithms()
	if file == nil || len(diags) > 0 {
		return
	}
	prevFile, prevImporting := c.file, c.importing
	prevDiags := len(c.info.Diagnostics)
	c.file = unit
	c.importing = "Swift"
	defer func() {
		c.file, c.importing = prevFile, prevImporting
		c.info.Diagnostics = c.info.Diagnostics[:prevDiags]
	}()

	decls := declsOf(file.Stmts)
	c.declareTypes(decls, scope)
	c.resolveTypeMembers(decls, scope)
	c.resolveExtensions(decls, scope)
	c.declareFunctions(decls, scope)
	for _, sym := range scope.Symbols() {
		if fn, ok := sym.(*FuncSymbol); ok {
			for _, f := range fn.Overloads() {
				c.info.Imported[f] = "Swift"
			}
		}
	}
	c.info.Scopes[file] = scope
	for _, stmt := range file.Stmts {
		c.checkStmt(stmt, scope)
	}
	c.info.CoreAlgorithms = file
}

// CoreDiagnostics is what the checker finds wrong with the core's
// algorithms, which loading the core keeps from a program's own
// diagnostics: the core's test reads them here.
func CoreDiagnostics() []token.Diagnostic {
	info := NewInfo()
	c := &checker{
		pg:        NewPrecedenceGraph(),
		info:      info,
		negated:   make(map[ast.Expr]bool),
		implicit:  make(map[ast.Expr]bool),
		declSites: make(map[Symbol]declSite),
		modules:   map[string]*Scope{},
	}
	universe := NewScope(nil, token.NoPos, token.NoPos)
	for _, typ := range types.Typ {
		if typ != nil && typ.Name() != "" {
			universe.Insert(NewTypeName(typ.Name(), typ, token.NoPos))
		}
	}
	for name, typ := range types.VertexAliases() {
		if typ != nil {
			universe.Insert(NewTypeName(name, typ, token.NoPos))
		}
	}
	for name, typ := range types.PointerNames() {
		universe.Insert(NewTypeName(name, typ, token.NoPos))
	}
	coreScope := NewScope(universe, token.NoPos, token.NoPos)
	file, unit, diags := core.Files()
	if file == nil || len(diags) > 0 {
		return diags
	}
	c.file = unit
	decls := declsOf(file.Stmts)
	c.declarePrecedenceAndOperators(decls)
	c.declareTypes(decls, coreScope)
	c.resolveTypeMembers(decls, coreScope)
	c.declareFunctions(decls, coreScope)
	for _, sym := range coreScope.Symbols() {
		if tn, ok := sym.(*TypeNameSymbol); ok && tn.Type() != nil {
			info.CoreTypes[tn.Type()] = true
		}
	}
	c.modules["Swift"] = coreScope
	alg, algUnit, algDiags := core.Algorithms()
	if alg == nil || len(algDiags) > 0 {
		return algDiags
	}
	c.file = algUnit
	c.importing = "Swift"
	decls = declsOf(alg.Stmts)
	c.declareTypes(decls, coreScope)
	c.resolveTypeMembers(decls, coreScope)
	c.resolveExtensions(decls, coreScope)
	c.declareFunctions(decls, coreScope)
	info.Scopes[alg] = coreScope
	for _, stmt := range alg.Stmts {
		c.checkStmt(stmt, coreScope)
	}
	return info.Diagnostics
}

// operatorSpelling is the text of an operator node.
func (c *checker) operatorSpelling(n ast.Node) string {
	if n == nil {
		return ""
	}
	return string(c.file.Slice(n.Pos(), n.End()))
}

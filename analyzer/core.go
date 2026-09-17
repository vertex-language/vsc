package analyzer

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
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

// operatorSpelling is the text of an operator node.
func (c *checker) operatorSpelling(n ast.Node) string {
	if n == nil {
		return ""
	}
	return string(c.file.Slice(n.Pos(), n.End()))
}

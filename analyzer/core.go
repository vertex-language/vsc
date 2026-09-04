package analyzer

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"github.com/vertex-language/vsc/types"
)

// The built-in module.
//
// Its declarations are read before the program's, into a scope the
// program's sits inside. That is the whole of the arrangement: an
// operator is a function core declares, `1 + 2` is a call to one, and
// the checker resolves it the way it resolves every other call rather
// than by a rule of its own.

// loadCore reads the built-in module into scope.
func (c *checker) loadCore(scope *Scope) {
	file, unit, diags := core.Files()
	if file == nil || len(diags) > 0 {
		// core does not parse. That is a fault in this compiler
		// rather than in the program being compiled, and core's own
		// test is where it is reported; here the program is checked
		// without it, which is what it did before core existed.
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

	// Whatever core had to say about itself is not the program's
	// concern.
	c.info.Diagnostics = c.info.Diagnostics[:prevDiags]
	c.file = prevFile
}

// builtinOperator finds the declaration an operator names, given what
// it is applied to.
//
// Overloads are resolved the way a call's are, because that is what
// this is: `a + b` is `+(a, b)`, and the declarations in core are
// what it picks from.
func (c *checker) builtinOperator(scope *Scope, op string, operands ...types.Type) *FuncSymbol {
	sym, _ := scope.Lookup(op).(*FuncSymbol)
	if sym == nil {
		return nil
	}
	for _, cand := range sym.Overloads() {
		sig := cand.Signature()
		if sig == nil || len(sig.Params) != len(operands) {
			continue
		}
		fits := true
		for i, t := range operands {
			if !types.AssignableTo(t, sig.Params[i].Type) {
				fits = false
				break
			}
		}
		if fits {
			return cand
		}
	}
	return nil
}

// operatorSpelling is the text of an operator node.
func (c *checker) operatorSpelling(n ast.Node) string {
	if n == nil {
		return ""
	}
	return string(c.file.Slice(n.Pos(), n.End()))
}

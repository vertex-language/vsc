package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// Calls that may fail.
//
// Swift carries the failure beside the result: the caller clears the
// error register before the call, the callee writes it on the path
// that fails, and the caller reads it after -- so one call has two
// edges out of it, which is what SILGen's `try_apply` says.
//
//	try_apply %f(%a), normal bb1, error bb2
//	bb1(%r : $Int32):
//	bb2(%e : $Error):
//
// What this compiler does with the error is nothing. `try?` discards
// it and produces an optional -- the value where the call did not
// fail, nothing where it did -- and that needs no error value at all.
// The rest is refused: `try` propagates the failure, and a `catch`
// that binds the error needs `any Error` -- an existential over a
// protocol the standard library declares, whose witness table this
// compiler would have to lay out the way libswiftCore does. That is
// the same problem as any other existential crossing a boundary, and
// the part still missing is the protocol descriptor a conformance
// points at.

// tryApply emits the two-edged call and the optional it produces.
func (g *gen) tryApply(e *ast.CallExpr, callee *vil.Value, args []*vil.Value,
	result types.Type, optional bool) *vil.Value {

	if !optional {
		g.errorAt(e, "cannot lower a call that may fail unless it is written 'try?': "+
			"what to do with the error needs 'any Error', whose conformance this "+
			"compiler names its own way")
		return nil
	}
	if isVoid(result) {
		g.refuse(e, "a call that may fail and returns nothing, whose 'try?' is an "+
			"optional of the empty tuple")
		return nil
	}

	opt := &types.Optional{Wrapped: result}
	normal := g.fn.Block()
	failed := g.fn.Block()
	join := g.fn.Block()
	value := normal.Arg(lowerType(result), vil.Owned)
	made := join.Arg(lowerType(opt), vil.Owned)

	g.blk.TryApply(callee, normal, failed, args...)

	// Where it did not fail: the value, wrapped.
	g.blk = normal
	some := g.blk.Enum(lowerType(opt), optionalSome, value)
	g.blk.Br(join, some)

	// Where it did: nothing.
	g.blk = failed
	none := g.blk.Enum(lowerType(opt), optionalNone, nil)
	g.blk.Br(join, none)

	g.blk = join
	return made
}

// tryExpr lowers `try`, `try?` and `try!`.
//
// Only `try?` and only on a call: the others need somewhere for the
// failure to go, which is a `catch` or the enclosing function's own
// error, and neither is lowered.
func (g *gen) tryExpr(e *ast.TryExpr) *vil.Value {
	if e.Question == token.NoPos {
		g.errorAt(e, "cannot lower 'try' unless it is written 'try?': what to do "+
			"with the error needs 'any Error', whose conformance this compiler "+
			"names its own way")
		return nil
	}
	call, ok := e.X.(*ast.CallExpr)
	if !ok {
		g.refuse(e, "'try?' on something other than a call")
		return nil
	}
	id, ok := call.Fun.(*ast.IdentExpr)
	if !ok || id.Name == nil {
		g.refuse(e, "'try?' on a call this compiler cannot name")
		return nil
	}
	sym, _ := g.info.Uses[id.Name].(*analyzer.FuncSymbol)
	if sym == nil {
		g.refuse(e, "'try?' on a call this compiler could not resolve")
		return nil
	}
	return g.callFuncTry(call, sym, true)
}

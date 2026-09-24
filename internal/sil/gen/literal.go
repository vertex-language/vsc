package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
)

// A literal written where a type expressible by it is wanted -- `let m:
// Money = 12`, the "a" of a Character -- is that type's literal
// initializer called with the literal, as the type the initializer takes:
// `Money(integerLiteral: 12)`. Swift's own literals go through the same
// protocols; here the core's types take theirs directly, and only a type
// that says it is expressible by one is made this way.

// literalInit lowers a literal the checker found is made by an
// initializer.
func (g *gen) literalInit(e ast.Expr, li *analyzer.LiteralInit) *sil.Value {
	if len(li.Init.Params) != 1 {
		g.refuse(e, "a literal whose initializer takes other than one value")
		return nil
	}
	if !isStructType(li.Type) {
		g.refuse(e, "a literal of a type that is not a struct")
		return nil
	}
	// The literal itself is what the initializer takes: an Int, a String.
	written := g.typeOf(e)
	g.info.Types[e] = li.Init.Params[0].Type
	delete(g.info.LiteralInits, e)
	raw := g.expr(e)
	g.info.Types[e] = written
	g.info.LiteralInits[e] = li
	if raw == nil {
		return nil
	}

	out := *li.Init
	out.Results = li.Type
	if g.isCoreType(li.Type) {
		g.emitCoreInit(li.Type, &out)
	}
	name := g.initSymbol(li.Type, &out)
	if name == "" {
		g.refuse(e, "this literal's initializer")
		return nil
	}
	ref := g.initRef(li.Type, &out, name)
	v := g.blk.Apply(ref, lowerType(li.Type), raw, g.blk.Metatype(lowerType(li.Type)))
	if _, isOpt := optionalOf(written); isOpt {
		v = g.optionalFor(e, v, li.Type, written)
	}
	g.destroyLater(v)
	return v
}

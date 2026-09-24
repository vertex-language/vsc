package gen

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// cast lowers the four casts.
//
//   - `x as T` is a coercion: a value put in an existential, or a literal
//     read as T, which the checker has already typed.
//   - `x as? T`, `x as! T` and `x is T` ask the runtime, as Swift does: it
//     looks inside an existential, unwraps an optional, wraps a value into
//     an Optional or an Any, and copies the value out through its witness.
func (g *gen) cast(e *ast.CastExpr) *sil.Value {
	from := g.typeOf(e.X)
	switch {
	case e.Kind == token.IS:
		return g.dynamicCast(e, from, g.castTarget(e), castIs)
	case e.Question.IsValid():
		return g.dynamicCast(e, from, g.castTarget(e), castOptional)
	case e.Exclaim.IsValid():
		return g.dynamicCast(e, from, g.typeOf(e), castForced)
	}
	to := g.typeOf(e)
	if _, isEx := existentialOf(to); isEx {
		if _, already := existentialOf(from); already {
			return g.ownedExistential(g.expr(e.X))
		}
		v := g.rvalue(e.X)
		if v == nil {
			return nil
		}
		return g.existentialFor(e.X, v, from, to)
	}
	v := g.rvalue(e.X)
	if v == nil {
		return nil
	}
	return g.optionalFor(e.X, v, from, to)
}

type castKind int

const (
	castOptional castKind = iota
	castForced
	castIs
)

// castTarget is the type a conditional cast or an `is` names.
func (g *gen) castTarget(e *ast.CastExpr) types.Type {
	if e.Kind == token.IS {
		if t, ok := g.info.CastTargets[e]; ok {
			return t
		}
		return nil
	}
	if o, ok := optionalOf(g.typeOf(e)); ok {
		return o.Wrapped
	}
	return nil
}

// dynamicCast asks the runtime whether x is a `to`, with a copy of it as
// one written where the answer is yes.
func (g *gen) dynamicCast(e *ast.CastExpr, from, to types.Type, kind castKind) *sil.Value {
	if from == nil || to == nil {
		g.refuse(e, "a cast whose types are not known")
		return nil
	}
	fromMeta, ok := g.stdlibMetadata(e, from)
	if !ok {
		return nil
	}
	toMeta, ok := g.stdlibMetadata(e, to)
	if !ok {
		return nil
	}
	// The source, in memory: an existential is there already; anything
	// else is put in a temporary.
	var src *sil.Value
	if _, isEx := existentialOf(from); isEx || isOptionalExistential(from) {
		src = g.existentialPlace(e.X)
		if src == nil {
			return nil
		}
	} else {
		v := g.rvalue(e.X)
		if v == nil {
			return nil
		}
		lt := lowerType(from)
		src = g.blk.AllocStack(lt)
		g.blk.Store(g.consume(v), src, storeQualifier(lt))
		if !lt.Trivial() {
			defer func() { g.destroyLater(g.blk.Load(src, "take")) }()
		}
	}
	lt := lowerType(to)
	out := g.blk.AllocStack(lt)
	raw := rawPointerType()
	matched := g.runtimeResult(stdlib.DynamicCast,
		[]sil.Param{{Type: raw, Convention: sil.ParamUnowned}, {Type: raw, Convention: sil.ParamUnowned},
			{Type: raw, Convention: sil.ParamUnowned}, {Type: raw, Convention: sil.ParamUnowned}},
		sil.Object(sil.BuiltinInt64),
		g.blk.AddressToPointer(out, raw), g.blk.AddressToPointer(src, raw), fromMeta, toMeta)
	bit := g.blk.Builtin("cmp_ne_Int64", sil.Object(sil.BuiltinInt1), matched,
		g.blk.IntegerLiteral(sil.Object(sil.BuiltinInt64), 0))

	loadOut := func() *sil.Value {
		if _, isEx := existentialOf(to); isEx {
			return out
		}
		return g.blk.Load(out, loadQualifierTake(lt))
	}

	switch kind {
	case castForced:
		failed := g.blk.Builtin("xor_Int1", sil.Object(sil.BuiltinInt1), bit,
			g.blk.IntegerLiteral(sil.Object(sil.BuiltinInt1), -1))
		g.blk.CondFail(failed, "Could not cast value to "+to.String())
		v := loadOut()
		if _, isEx := existentialOf(to); isEx {
			return v
		}
		g.destroyLater(v)
		return v

	case castIs:
		yes, no, join := g.fn.Block(), g.fn.Block(), g.fn.Block()
		answer := join.Arg(sil.Object(sil.BuiltinInt1), sil.None)
		g.blk.CondBr(bit, yes, nil, no, nil)
		g.blk = yes
		if _, isEx := existentialOf(to); isEx {
			g.blk.DestroyAddr(out)
		} else if !lt.Trivial() {
			g.blk.DestroyValue(g.blk.Load(out, "take"))
		}
		g.blk.Br(join, g.blk.IntegerLiteral(sil.Object(sil.BuiltinInt1), -1))
		g.blk = no
		g.blk.Br(join, g.blk.IntegerLiteral(sil.Object(sil.BuiltinInt1), 0))
		g.blk = join
		return g.boolOf(answer)
	}

	opt := &types.Optional{Wrapped: to}
	if _, isEx := existentialOf(to); isEx {
		g.refuse(e, "an 'as?' to an existential")
		return nil
	}
	optType := lowerType(opt)
	yes, no, join := g.fn.Block(), g.fn.Block(), g.fn.Block()
	g.blk.CondBr(bit, yes, nil, no, nil)
	g.blk = yes
	g.blk.Br(join, g.blk.Enum(optType, optionalSome, g.blk.Load(out, loadQualifierTake(lt))))
	g.blk = no
	g.blk.Br(join, g.blk.Enum(optType, optionalNone, nil))
	g.blk = join
	own := sil.Owned
	if optType.Trivial() {
		own = sil.Unowned
	}
	result := join.Arg(optType, own)
	g.destroyLater(result)
	return result
}

// loadQualifierTake is how a value is moved out of memory.
func loadQualifierTake(t sil.Type) string {
	if t.Trivial() {
		return "trivial"
	}
	return "take"
}

// castValue asks the runtime whether v, of type from -- a value, or an
// existential's storage -- is a `to`, answering the bit and the stack slot
// a copy of it as one is written to where it is. The caller takes that
// copy out and deallocates the slot.
func (g *gen) castValue(at ast.Node, v *sil.Value, from, to types.Type) (*sil.Value, *sil.Value, bool) {
	if from == nil || to == nil {
		g.refuse(at, "a cast whose types are not known")
		return nil, nil, false
	}
	fromMeta, ok := g.stdlibMetadata(at, from)
	if !ok {
		return nil, nil, false
	}
	toMeta, ok := g.stdlibMetadata(at, to)
	if !ok {
		return nil, nil, false
	}
	src := v
	var temp *sil.Value
	if !v.Type().IsAddress() {
		lt := lowerType(from)
		temp = g.blk.AllocStack(lt)
		held := v
		if !lt.Trivial() {
			held = g.blk.CopyValue(v)
		}
		g.blk.Store(held, temp, storeQualifier(lt))
		src = temp
	}
	out := g.blk.AllocStack(lowerType(to))
	raw := rawPointerType()
	matched := g.runtimeResult(stdlib.DynamicCast,
		[]sil.Param{{Type: raw, Convention: sil.ParamUnowned}, {Type: raw, Convention: sil.ParamUnowned},
			{Type: raw, Convention: sil.ParamUnowned}, {Type: raw, Convention: sil.ParamUnowned}},
		sil.Object(sil.BuiltinInt64),
		g.blk.AddressToPointer(out, raw), g.blk.AddressToPointer(src, raw), fromMeta, toMeta)
	if temp != nil {
		if lt := lowerType(from); !lt.Trivial() {
			g.blk.DestroyAddr(temp)
		}
		g.blk.DeallocStack(temp)
	}
	bit := g.blk.Builtin("cmp_ne_Int64", sil.Object(sil.BuiltinInt1), matched,
		g.blk.IntegerLiteral(sil.Object(sil.BuiltinInt64), 0))
	return out, bit, true
}

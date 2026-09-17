package gen

import (
	"unicode/utf8"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/types"
)

// String literal and interpolation lowering.

// stringLiteralSymbol is the runtime's vertex_string_literal.
const stringLiteralSymbol = stdlib.StringLiteral

// stringLiteral builds the String a literal denotes.
func (g *gen) stringLiteral(e *ast.StringLit) *sil.Value {
	var s *sil.Value
	v, ok := g.info.Values[e]
	if !ok || v.Kind != analyzer.StringValue {
		// Several pieces and a concatenation. See interpolated.
		s = g.interpolated(e)
	} else {
		s = g.makeString(v.Str)
	}
	// A literal written where a `String?` is wanted takes that type, and
	// is the String it spells wrapped in the case that holds one.
	if s != nil {
		if t := g.typeOf(e); t != nil {
			if _, isOpt := optionalOf(t); isOpt {
				g.forget(s)
				wrapped := g.blk.Enum(lowerType(t), optionalSome, s)
				g.destroyLater(wrapped)
				return wrapped
			}
		}
	}
	return s
}

// makeString builds the String some run of text denotes.
func (g *gen) makeString(text string) *sil.Value {
	t := types.Type(types.Typ[types.String])

	callee := g.m.Func(stringLiteralSymbol).SetSourceName("String.init")
	if g.needsType(callee) {
		callee.SetLinkage(sil.PublicExternal)
		ft := callee.Type()
		// String initializer takes (raw pointer, length, isASCII).
		ft.Convention = sil.Thin
		ft.Params = []sil.Param{
			{Type: sil.Object(sil.BuiltinRawPointer)},
			{Type: sil.Object(sil.BuiltinWord)},
			{Type: sil.Object(sil.BuiltinInt1)},
		}
		callee.SetResult(lowerType(t), sil.ResultOwned)
	}

	bytes := text
	ascii := int64(0)
	if isASCII(bytes) {
		// Swift's Builtin.Int1 true is all ones, which is what SILGen
		// writes and what one bit of it reads back as.
		ascii = -1
	}
	ref := g.blk.FunctionRef(callee)
	s := g.blk.Apply(ref, lowerType(t),
		g.blk.StringLiteral(bytes, "utf8"),
		g.blk.IntegerLiteral(sil.Object(sil.BuiltinWord), int64(len(bytes))),
		g.blk.IntegerLiteral(sil.Object(sil.BuiltinInt1), ascii))
	g.destroyLater(s)
	return s
}

// isString reports whether a type is Swift's String.
func isString(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Kind() == types.String
}

// isASCII reports whether every byte in s is ASCII.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// externOperator lowers an operator implemented as a standard library call.
func (g *gen) externOperator(at ast.Node, op string, operandType, results types.Type,
	ex core.Extern, lhs, rhs *sil.Value) *sil.Value {
	operand := lowerType(operandType)
	result := lowerType(results)

	callee := g.m.Func(ex.Symbol).SetSourceName(op)
	if g.needsType(callee) {
		callee.SetLinkage(sil.PublicExternal)
		ft := callee.Type()
		ft.Convention = sil.Thin
		ft.Params = []sil.Param{
			{Type: operand, Convention: sil.ParamGuaranteed},
			{Type: operand, Convention: sil.ParamGuaranteed},
		}
		conv := sil.ResultUnowned
		if ex.Owned {
			conv = sil.ResultOwned
		}
		callee.SetResult(result, conv)
	}
	v := g.blk.Apply(g.blk.FunctionRef(callee), result, lhs, rhs)
	if ex.Owned {
		g.destroyLater(v)
	}
	if !ex.Negate {
		return v
	}
	// Invert the boolean result for negated operators.
	bit := g.machine(v, results)
	if bit == nil {
		return nil
	}
	flipped := g.blk.Builtin("xor_Int1", sil.Object(sil.BuiltinInt1),
		bit, g.blk.IntegerLiteral(sil.Object(sil.BuiltinInt1), -1))
	return g.blk.Struct(result, flipped)
}

// externCompare emits a comparison call and unwraps the resulting Bool to an Int1 bit.
func (g *gen) externCompare(a, b *sil.Value, t types.Type, ex core.Extern) *sil.Value {
	boolean := types.Typ[types.Bool]
	callee := g.m.Func(ex.Symbol).SetSourceName("==")
	if g.needsType(callee) {
		callee.SetLinkage(sil.PublicExternal)
		ft := callee.Type()
		ft.Convention = sil.Thin
		ft.Params = []sil.Param{
			{Type: lowerType(t), Convention: sil.ParamGuaranteed},
			{Type: lowerType(t), Convention: sil.ParamGuaranteed},
		}
		callee.SetResult(lowerType(boolean), sil.ResultUnowned)
	}
	v := g.blk.Apply(g.blk.FunctionRef(callee), lowerType(boolean), a, b)
	bit := g.machine(v, boolean)
	if bit == nil || !ex.Negate {
		return bit
	}
	return g.blk.Builtin("xor_Int1", sil.Object(sil.BuiltinInt1),
		bit, g.blk.IntegerLiteral(sil.Object(sil.BuiltinInt1), -1))
}

// stdlibGetter reads a property of a String or Array via its runtime getter function.
func (g *gen) stdlibGetter(e *ast.MemberExpr, m core.Member) *sil.Value {
	recv := g.expr(e.X)
	if recv == nil {
		return nil
	}
	return g.readProperty(e, recv, g.typeOf(e.X), m, g.text(e.Name))
}

// readProperty invokes a runtime property getter on an already lowered receiver.
func (g *gen) readProperty(at ast.Node, recv *sil.Value, recvType types.Type,
	m core.Member, name string) *sil.Value {
	var meta *sil.Value
	if m.Element != nil {
		var ok bool
		meta, ok = g.stdlibMetadata(at, m.Element)
		if !ok {
			return nil
		}
	}
	self := lowerType(recvType)
	result := lowerType(m.Result)

	callee := g.m.Func(m.Symbol).SetSourceName(name)
	if g.needsType(callee) {
		callee.SetLinkage(sil.PublicExternal)
		ft := callee.Type()
		ft.Convention = sil.Thin
		ft.Params = []sil.Param{{Type: self, Convention: sil.ParamGuaranteed}}
		if meta != nil {
			ft.Params = append(ft.Params,
				sil.Param{Type: sil.Object(sil.BuiltinRawPointer)})
		}
		callee.SetResult(result, resultConvention(result))
	}
	args := []*sil.Value{recv}
	if meta != nil {
		args = append(args, meta)
	}
	v := g.blk.Apply(g.blk.FunctionRef(callee), result, args...)
	g.destroyLater(v)
	return v
}

// describingSymbol is `String(describing:)`, which the runtime
// implements over the value's metadata.
const describingSymbol = stdlib.Describe

// interpolated builds a concatenated string from text segments and String(describing:) interpolations.
func (g *gen) interpolated(e *ast.StringLit) *sil.Value {
	var acc *sil.Value
	for _, seg := range e.Segments {
		var piece *sil.Value
		switch n := seg.(type) {
		case *ast.StringText:
			v, ok := g.info.Values[n]
			if !ok || v.Kind != analyzer.StringValue {
				g.refuse(e, "a run of text in a literal this compiler did not read")
				return nil
			}
			if v.Str == "" {
				continue
			}
			piece = g.makeString(v.Str)
		case *ast.Interpolation:
			if n.X == nil {
				g.refuse(n, "an interpolation with more than an expression in it")
				return nil
			}
			piece = g.describing(n, n.X)
		default:
			continue
		}
		if piece == nil {
			return nil
		}
		if acc == nil {
			acc = piece
			continue
		}
		acc = g.concatenate(e, acc, piece)
		if acc == nil {
			return nil
		}
	}
	if acc == nil {
		return g.makeString("")
	}
	return acc
}

// describing emits a call to String(describing:) with the value's address and type metadata.
func (g *gen) describing(at ast.Node, e ast.Expr) *sil.Value {
	t := g.typeOf(e)
	if t == nil {
		g.refuse(at, "an interpolation whose type is not known")
		return nil
	}
	// A type that says what it is as text is described by that.
	if f, ok := g.descriptionOf(t); ok {
		return g.getterCall(at, t, f, func() *sil.Value { return g.expr(e) })
	}
	meta, ok := g.stdlibMetadata(at, t)
	if !ok {
		return nil
	}
	v := g.rvalue(e)
	if v == nil {
		return nil
	}
	slot := g.blk.AllocStack(lowerType(t))
	g.blk.Store(v, slot, storeQualifier(lowerType(t)))

	str := lowerType(types.Typ[types.String])
	callee := g.m.Func(describingSymbol).SetSourceName("String.init")
	if g.needsType(callee) {
		callee.SetLinkage(sil.PublicExternal)
		ft := callee.Type()
		ft.Convention = sil.Thin
		ft.Params = []sil.Param{
			{Type: lowerType(t).Address(), Convention: sil.ParamInGuaranteed},
			{Type: sil.Object(sil.BuiltinRawPointer)},
		}
		callee.SetResult(str, sil.ResultOwned)
	}
	out := g.blk.Apply(g.blk.FunctionRef(callee), str, slot, meta)
	g.destroyLater(out)
	// Destroy the temporary copy after the call.
	if lt := lowerType(t); !lt.Trivial() {
		g.destroyLater(g.blk.Load(slot, "take"))
	}
	return out
}

// concatenate joins two pieces of an interpolated literal.
func (g *gen) concatenate(at ast.Node, a, b *sil.Value) *sil.Value {
	str := types.Type(types.Typ[types.String])
	ex, ok := core.LowerExtern("+", str)
	if !ok {
		g.refuse(at, "a concatenation this compiler cannot name")
		return nil
	}
	return g.externOperator(at, "+", str, str, ex, a, b)
}

// runtimeCall emits a call to a nullary runtime function returning an owned value of type t.
func (g *gen) runtimeCall(symbol string, t sil.Type) *sil.Value {
	callee := g.m.Func(symbol).SetSourceName(symbol)
	if g.needsType(callee) {
		callee.SetLinkage(sil.PublicExternal)
		callee.Type().Convention = sil.Thin
		callee.SetResult(t, resultConvention(t))
	}
	v := g.blk.Apply(g.blk.FunctionRef(callee), t)
	g.destroyLater(v)
	return v
}

// descriptionOf is the description property of a type that conforms to
// CustomStringConvertible, which is what describing a value of it shows.
//
// It is found from the type the value has where it is described. A value
// already inside an Any, an array or an optional is described by the
// runtime from its metadata, which does not look conformances up yet, and
// shows its stored properties instead.
func (g *gen) descriptionOf(t types.Type) (*types.Field, bool) {
	if t == nil || !g.conformsByName(t, "CustomStringConvertible") {
		return nil, false
	}
	f, ok := g.computedField(t, "description")
	if !ok || !types.Identical(f.Type, types.Typ[types.String]) {
		return nil, false
	}
	return f, true
}

// conformsByName reports whether a nominal type declares a conformance to
// the protocol of that name, or to one inheriting it.
func (g *gen) conformsByName(t types.Type, name string) bool {
	var walk func(ps []*types.Protocol) bool
	walk = func(ps []*types.Protocol) bool {
		for _, p := range ps {
			if p != nil && (p.Name == name || walk(p.Inherited)) {
				return true
			}
		}
		return false
	}
	return walk(g.conformancesOf(t))
}

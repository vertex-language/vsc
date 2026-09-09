package gen

import (
	"unicode/utf8"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// A string literal.
//
// Two words come out of it, and what goes in them is Swift's business
// rather than this compiler's. `"hi"` is the bytes themselves in the
// registers; `"abcdefghijklmnop"` is a pointer to bytes in the object
// file, biased by thirty-two and with the immortal bit set; a
// non-ASCII one is neither of those spellings exactly. All three are
// what one initializer in the standard library returns, so that is
// what this calls -- which is also what SILGen emits, down to the
// three operands:
//
//	%0 = string_literal utf8 "abcdefghijklmnop"
//	%1 = integer_literal $Builtin.Word, 16
//	%2 = integer_literal $Builtin.Int1, -1
//	%5 = apply %4(%0, %1, %2, %3)
//
// Reimplementing the encoding here would mean writing down the small
// form's discriminator byte, the large form's bias and its flag bits,
// and keeping all of it right as the standard library changes them.
// Calling the initializer means agreeing with whatever they are.

// stringLiteralSymbol is
// String.init(_builtinStringLiteral:utf8CodeUnitCount:isASCII:),
// which libswiftCore exports.
const stringLiteralSymbol = "$sSS21_builtinStringLiteral17utf8CodeUnitCount7isASCIISSBp_BwBi1_tcfC"

// stringLiteral builds the String a literal denotes.
func (g *gen) stringLiteral(e *ast.StringLit) *vil.Value {
	v, ok := g.info.Values[e]
	if !ok || v.Kind != analyzer.StringValue {
		// Several pieces and a concatenation. See interpolated.
		return g.interpolated(e)
	}
	return g.makeString(v.Str)
}

// makeString builds the String some run of text denotes.
func (g *gen) makeString(text string) *vil.Value {
	t := types.Type(types.Typ[types.String])

	callee := g.m.Func(stringLiteralSymbol).SetSourceName("String.init")
	if g.needsType(callee) {
		callee.SetLinkage(vil.PublicExternal)
		ft := callee.Type()
		// The metatype the initializer also takes is thin -- it names
		// String and carries nothing -- so it is no argument at all.
		// swiftc's own call passes the pointer in x0, the count in x1
		// and the ASCII flag in w2, and nothing else.
		ft.Convention = vil.Thin
		ft.Params = []vil.Param{
			{Type: vil.Object(vil.BuiltinRawPointer)},
			{Type: vil.Object(vil.BuiltinWord)},
			{Type: vil.Object(vil.BuiltinInt1)},
		}
		callee.SetResult(lowerType(t), vil.ResultOwned)
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
		g.blk.IntegerLiteral(vil.Object(vil.BuiltinWord), int64(len(bytes))),
		g.blk.IntegerLiteral(vil.Object(vil.BuiltinInt1), ascii))
	g.destroyLater(s)
	return s
}

// isString reports whether a type is Swift's String.
func isString(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Kind() == types.String
}

// isASCII reports whether every byte of a literal is one, which is
// what the initializer's third argument says. It is a fact about the
// bytes and cheaper to state than to rediscover.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// externOperator lowers an operator whose implementation is a
// standard-library call rather than a machine instruction.
//
// Which is every operator on String. `a + b` allocates and `a == b`
// walks two sequences of bytes that may not be encoded the same way;
// neither is an instruction, and both are already written. So this is
// a call, and the shape is the one swiftc's own code has: both
// operands guaranteed, the thin metatype no register at all, and the
// result owned where it is a String.
func (g *gen) externOperator(at ast.Node, op string, operandType, results types.Type,
	ex core.Extern, lhs, rhs *vil.Value) *vil.Value {
	operand := lowerType(operandType)
	result := lowerType(results)

	callee := g.m.Func(ex.Symbol).SetSourceName(op)
	if g.needsType(callee) {
		callee.SetLinkage(vil.PublicExternal)
		ft := callee.Type()
		ft.Convention = vil.Thin
		ft.Params = []vil.Param{
			{Type: operand, Convention: vil.ParamGuaranteed},
			{Type: operand, Convention: vil.ParamGuaranteed},
		}
		conv := vil.ResultUnowned
		if ex.Owned {
			conv = vil.ResultOwned
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
	// The negation of the answer, which is what `!=` is: reach through
	// the Bool to its bit, flip it, and wrap it again.
	bit := g.machine(v, results)
	if bit == nil {
		return nil
	}
	flipped := g.blk.Builtin("xor_Int1", vil.Object(vil.BuiltinInt1),
		bit, g.blk.IntegerLiteral(vil.Object(vil.BuiltinInt1), -1))
	return g.blk.Struct(result, flipped)
}

// externCompare is the bit an ordering or equality test on a String
// produces, where the test is a call.
//
// It is externOperator with the operands already lowered and the Bool
// already taken apart, which is what a switch's case test wants: the
// bit, rather than the Bool around it.
func (g *gen) externCompare(a, b *vil.Value, t types.Type, ex core.Extern) *vil.Value {
	boolean := types.Typ[types.Bool]
	callee := g.m.Func(ex.Symbol).SetSourceName("==")
	if g.needsType(callee) {
		callee.SetLinkage(vil.PublicExternal)
		ft := callee.Type()
		ft.Convention = vil.Thin
		ft.Params = []vil.Param{
			{Type: lowerType(t), Convention: vil.ParamGuaranteed},
			{Type: lowerType(t), Convention: vil.ParamGuaranteed},
		}
		callee.SetResult(lowerType(boolean), vil.ResultUnowned)
	}
	v := g.blk.Apply(g.blk.FunctionRef(callee), lowerType(boolean), a, b)
	bit := g.machine(v, boolean)
	if bit == nil || !ex.Negate {
		return bit
	}
	return g.blk.Builtin("xor_Int1", vil.Object(vil.BuiltinInt1),
		bit, g.blk.IntegerLiteral(vil.Object(vil.BuiltinInt1), -1))
}

// stdlibGetter reads a property of a String or an Array through the
// getter libswiftCore exports for it.
//
// The shape is swiftc's: the receiver guaranteed, the result in a
// register, and -- where the getter is generic -- the metadata for
// the element type after it. swiftc's own `a.count` for an [Int32]
// puts the array in x0 and Int32's metadata in x1.
func (g *gen) stdlibGetter(e *ast.MemberExpr, m core.Member) *vil.Value {
	recv := g.expr(e.X)
	if recv == nil {
		return nil
	}
	return g.readProperty(e, recv, g.typeOf(e.X), m, g.text(e.Name))
}

// readProperty is stdlibGetter with the receiver already lowered,
// which is what a for-in over an array wants: the sequence is
// evaluated once and its count read from the value, not from an
// expression the source could write twice.
func (g *gen) readProperty(at ast.Node, recv *vil.Value, recvType types.Type,
	m core.Member, name string) *vil.Value {
	var meta *vil.Value
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
		callee.SetLinkage(vil.PublicExternal)
		ft := callee.Type()
		ft.Convention = vil.Thin
		ft.Params = []vil.Param{{Type: self, Convention: vil.ParamGuaranteed}}
		if meta != nil {
			ft.Params = append(ft.Params,
				vil.Param{Type: vil.Object(vil.BuiltinRawPointer)})
		}
		callee.SetResult(result, resultConvention(result))
	}
	args := []*vil.Value{recv}
	if meta != nil {
		args = append(args, meta)
	}
	v := g.blk.Apply(g.blk.FunctionRef(callee), result, args...)
	g.destroyLater(v)
	return v
}

// describingSymbol is `String.init<Subject>(describing:)`, which is
// the one the standard library exports without a constraint on what
// it is given.
const describingSymbol = "$sSS10describingSSx_tclufC"

// interpolated builds the String a literal with `\(…)` in it denotes.
//
// The pieces and a concatenation. SILGen writes something else -- a
// DefaultStringInterpolation, an appendLiteral per run of text, and
// an appendInterpolation per hole -- and every appendInterpolation
// that does anything useful is constrained by a protocol the standard
// library declares, which is a witness table this compiler does not
// lay out.
//
// What it emits instead is `String(describing:)` per hole, which is
// the unconstrained one, and `+` between the pieces. That is not a
// guess about what the two produce: the constrained overloads write
// `description` and the unconstrained one calls the same code
// `String(describing:)` does, so the text is the same -- and the test
// that says so builds the same program with both compilers and
// compares what they print.
func (g *gen) interpolated(e *ast.StringLit) *vil.Value {
	var acc *vil.Value
	for _, seg := range e.Segments {
		var piece *vil.Value
		switch n := seg.(type) {
		case *ast.StringText:
			v, ok := g.info.Values[n]
			if !ok || v.Kind != analyzer.StringValue {
				g.refuse(e, "a run of text in a literal this compiler did not read")
				return nil
			}
			if v.Str == "" {
				// An empty run adds nothing, and the two the
				// delimiters leave at each end are usually empty.
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
		// A literal of nothing but empty runs is the empty string.
		return g.makeString("")
	}
	return acc
}

// describing is `String(describing: x)`: the value handed over by
// address, because the callee is generic and does not know how big it
// is, and the metadata for its type after it.
func (g *gen) describing(at ast.Node, e ast.Expr) *vil.Value {
	t := g.typeOf(e)
	if t == nil {
		g.refuse(at, "an interpolation whose type is not known")
		return nil
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
		callee.SetLinkage(vil.PublicExternal)
		ft := callee.Type()
		ft.Convention = vil.Thin
		ft.Params = []vil.Param{
			{Type: lowerType(t).Address(), Convention: vil.ParamInGuaranteed},
			{Type: vil.Object(vil.BuiltinRawPointer)},
		}
		callee.SetResult(str, vil.ResultOwned)
	}
	out := g.blk.Apply(g.blk.FunctionRef(callee), str, slot, meta)
	g.destroyLater(out)
	return out
}

// concatenate joins two pieces of an interpolated literal.
func (g *gen) concatenate(at ast.Node, a, b *vil.Value) *vil.Value {
	str := types.Type(types.Typ[types.String])
	ex, ok := core.LowerExtern("+", str)
	if !ok {
		g.refuse(at, "a concatenation this compiler cannot name")
		return nil
	}
	return g.externOperator(at, "+", str, str, ex, a, b)
}
